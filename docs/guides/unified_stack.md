# Unified Docker Stack Guide

Last Updated: 2026-09-08
Version: v2.1.7

This guide explains how to run the gateway, operator, ensemble (g8ee), and dashboard (g8ed) from the repository root as one Docker Compose stack. You can manage the stack with Docker Compose directly or with the `./g8e docker` commands.

## Prerequisites

The unified stack requires:

- Docker Engine with the Docker Compose v2 plugin.
- The g8e repository and `./g8e` binary on the host.
- Ports 8080, 8443, 8000, and 3000 available when using the defaults.
- A browser with WebAuthn support for interactive owner enrollment. Headless enrollment is available when no browser is present.

Run all commands in this guide from the repository root. The `./g8e docker` commands require `docker-compose.yml` in the current directory.

The start and reset commands run `docker compose up -d` without `--build`. Build the images after source changes or when you want to guarantee a fresh local image:

```bash
./g8e docker build
```

You can also use `docker compose up -d --build` in the manual workflow.

## Stack Services

The root `docker-compose.yml` defines four services on the `g8e-net` bridge network:

| Service | Build | Published ports | Role |
| --- | --- | --- | --- |
| `g8e-gateway` | Root `Dockerfile` | 8080 HTTP, 8443 HTTPS | Policy Decision Point. It admits transactions, manages PKI, enforces gateway governance, brokers pub/sub, and serves the console, MCP, and A2A surfaces. |
| `g8e-operator` | Root `Dockerfile` | None | Policy Execution Point. It connects outbound to the gateway over mTLS, receives work, re-verifies proofs, and performs L4 and L5 execution. |
| `ensemble` | `ensemble/Dockerfile` with the repository root as its build context | 8000 | First-party agentic ensemble. It runs the AI reasoning flow, submits governed transactions, and publishes events. See the [g8ee documentation](../ensemble/index.md). |
| `dashboard` | `dashboard/Dockerfile` with the repository root as its build context | 3000 | First-party browser interface for chat, operator management, audit, and settings. See the [g8ed documentation](../dashboard/index.md). |

The gateway and operator use the same Go image and binary. The Linux AMD64 binary includes the Go FIPS 140-3 cryptographic module. Strict runtime enforcement is off by default because the platform uses cryptographic primitives that strict mode rejects. See the [Docker Gateway Guide](./docker_gateway.md) for the supported operating environment and verification details.

## Startup Model

Only `g8e-gateway` belongs to the default Compose profile. The operator, ensemble, and dashboard belong to the `bootstrapped` profile. A fresh gateway initializes its PKI immediately, but it starts with no users and does not issue workload credentials until the first owner enrolls and approves each platform enrollment request.

The profile controls which containers Compose starts; it does not bypass enrollment. The three workload containers remain not ready while they wait for owner approval.

## Automated Workflow

The interactive helper starts all four services and walks through enrollment using the default host ports:

```bash
./g8e docker start --full
```

On a fresh deployment, the command:

1. Starts the `bootstrapped` Compose profile in the background.
2. Polls `http://127.0.0.1:8080/api/v1/health` until the gateway responds.
3. Enrolls the local CLI identity. The first enrollment creates the owner and registers a WebAuthn passkey; an existing valid identity is reused.
4. Checks for pending enrollment requests in ensemble, dashboard, then operator order and prompts before approving each request.

Each approval prompt accepts `y` or `yes`; any other response skips that component. The walkthrough checks each component only once. If a container has not submitted its request by the time it is checked, the command reports that no request was found and continues. Use the manual approval commands below for any request that appears later.

The helper assumes the default gateway host ports, 8080 and 8443. Use the manual workflow when the gateway is published on different ports.

To start all workload containers without running owner enrollment or approval prompts:

```bash
./g8e docker start --full --skip-enroll
```

The workloads remain pending until an enrolled owner approves them.

## Manual Workflow

### 1. Start the gateway

```bash
docker compose up -d --build
until curl -fsS http://localhost:8080/api/v1/health >/dev/null 2>&1; do sleep 2; done
```

This starts only `g8e-gateway` because the other services are in the `bootstrapped` profile.

### 2. Enroll the first owner

```bash
./g8e auth enroll user -e localhost
```

The discovery endpoint uses HTTP port 8080 and the authenticated API uses HTTPS port 8443 by default. Interactive enrollment installs the gateway root CA into system trust before opening the browser for passkey registration. The command prompts before operations that require user action.

If local credentials remain from a gateway whose volumes were removed, the enrollment coordinator checks the new gateway state and performs initial bootstrap when no owner exists.

### 3. Start the workload containers

```bash
docker compose --profile bootstrapped up -d
```

Each workload submits its own enrollment request and waits for a decision. Requests can appear at different times.

### 4. Approve the workload requests

```bash
./g8e auth pending-platform-enrollments
```

Approve each exact request ID. Approving the operator first makes its transport identity available before the ensemble begins governed submissions:

```bash
./g8e auth approve-platform-enrollment <operator-request-id> --yes
./g8e auth approve-platform-enrollment <dashboard-request-id> --yes
./g8e auth approve-platform-enrollment <ensemble-request-id> --yes
```

Without `--yes`, each command displays request metadata and asks for confirmation. Re-run `pending-platform-enrollments` if a component has not submitted its request yet. You can deny a request with `--deny` and optionally include `--reason`.

You can also approve pending requests in the gateway console at `https://localhost:8443/console/`. Console authentication uses the owner's WebAuthn passkey.

### 5. Verify readiness

```bash
docker compose --profile bootstrapped ps
```

When all enrollment flows complete, the following surfaces are available:

- Gateway discovery and CA bundle: `http://localhost:8080`
- Gateway HTTPS API: `https://localhost:8443`
- Gateway console: `https://localhost:8443/console/`
- Ensemble API: `http://localhost:8000`
- Dashboard: `http://localhost:3000`

The gateway starts in `doctrine` posture. Compose configures the dashboard origin for CORS and WebAuthn, so the browser SPA can establish a passkey-backed gateway session and call the gateway directly.

## Non-Default Ports and Hostnames

The compose file reads six optional environment variables. The repository root `.env.example` contains the same defaults.

| Variable | Default | Effect |
| --- | --- | --- |
| `G8E_PREFIX` | `g8e` | Prefixes container names such as `${G8E_PREFIX}-gateway`. It does not rename the Compose network or named volumes. |
| `G8E_HTTP_PORT` | `8080` | Host port for gateway discovery, health, CA bundle retrieval, and enrollment submission. |
| `G8E_HTTPS_PORT` | `8443` | Host port for the gateway HTTPS and mTLS API. |
| `G8E_ENSEMBLE_PORT` | `8000` | Host port for the ensemble API. |
| `G8E_DASHBOARD_PORT` | `3000` | Host port for the dashboard. |
| `G8E_HOSTNAME` | `localhost` | Browser-visible gateway hostname used in the public URL, CORS origin, and WebAuthn relying-party configuration. |

Export overrides so they apply to every Compose command in the workflow:

```bash
export G8E_HTTP_PORT=18080
export G8E_HTTPS_PORT=18443
export G8E_ENSEMBLE_PORT=18000
export G8E_DASHBOARD_PORT=13000
docker compose up -d --build
```

Pass both gateway ports to host-side authentication commands:

```bash
./g8e auth enroll user -e localhost:18080 --port 18443
docker compose --profile bootstrapped up -d
./g8e auth pending-platform-enrollments -e localhost:18080 --port 18443
./g8e auth approve-platform-enrollment <request-id> --yes -e localhost:18080 --port 18443
```

Set `G8E_HOSTNAME` to the hostname used by the browser when accessing the gateway from another machine. The hostname must resolve to the Docker host, and the published ports must be reachable. WebAuthn relying-party IDs are hostnames, not URLs or host-and-port strings.

## CLI Management

| Command | Behavior |
| --- | --- |
| `./g8e docker start` | Starts the default profile, which contains only the gateway. |
| `./g8e docker start --full` | Starts the `bootstrapped` profile and runs the interactive enrollment walkthrough. |
| `./g8e docker start --full --skip-enroll` | Starts all services without the enrollment walkthrough. |
| `./g8e docker start --profile <name>` | Starts an explicit Compose profile. A non-empty profile also runs the walkthrough unless `--skip-enroll` is set. |
| `./g8e docker stop` | Runs `docker compose down` for all unified-stack services. It removes containers and the Compose network but preserves named volumes. |
| `./g8e docker status` | Runs `docker compose ps`. |
| `./g8e docker build` | Builds the images used by all four services. `--no-cache` disables the build cache. |
| `./g8e docker logs [service] [-f]` | Prints logs for the stack or one Compose service and optionally follows them. |
| `./g8e docker rebuild [--full]` | Stops and rebuilds the explicit `--profile`, or the default profile when none is given, then restarts the profile selected by `--profile` or `--full`. It preserves volumes and does not run enrollment. The build bypasses the cache by default; pass `--no-cache=false` to reuse it. |
| `./g8e docker reset [--full]` | Runs destructive cleanup and restarts the selected scope without rebuilding images or running enrollment. It does not ask for confirmation. `--full` selects the restart profile but does not select that profile for the cleanup step. |
| `./g8e docker clean` | Removes containers, volumes, networks, and orphans across the unified stack. It skips confirmation by default; pass `--yes=false` to require a prompt. |

Use `--profile bootstrapped` with `rebuild` or `reset` when the operation must target the full profile during both shutdown and restart. A successful reset destroys state in the volumes it removes. `clean` always targets the full profile and destroys the gateway PKI, owner records, audit data, operator identity, and app identities. Use these destructive commands only when a fresh trust domain is intended.

## Dependencies, Health Checks, and Resources

The operator, ensemble, and dashboard wait for the gateway container to become healthy. The ensemble also waits for the operator container to start, but not for operator enrollment to complete.

Compose uses these health checks:

- `g8e-gateway`: HTTP `GET /api/v1/health` on port 8080.
- `g8e-operator`: presence of the enrolled operator certificate.
- `ensemble`: HTTP `GET /health` on port 8000. FastAPI does not accept this request until startup enrollment and service initialization complete.
- `dashboard`: HTTP `GET /` on port 3000. Express starts listening only after dashboard app enrollment completes.

The workload health checks therefore remain unhealthy or in their startup period while enrollment is pending.

| Service | CPU limit | Memory limit | CPU reservation | Memory reservation |
| --- | --- | --- | --- | --- |
| `g8e-gateway` | 2 | 1G | 0.5 | 256M |
| `g8e-operator` | 2 | 1G | 0.5 | 256M |
| `ensemble` | 2 | 2G | 0.5 | 512M |
| `dashboard` | 1 | 512M | 0.25 | 128M |

## PKI and Workload Identity

The gateway creates its CA hierarchy, serving certificate, and canonical trust bundle in the `g8e-gateway-data` volume. It serves the CA bundle from `http://<gateway>:8080/.well-known/g8e/pki/ca-bundle`. Read-only host identity mounts allow the gateway to include detected host addresses and names in its serving certificate and regenerate that certificate when its SAN set changes.

The operator, ensemble, and dashboard each complete the owner-approved platform enrollment protocol and persist their credentials in separate named volumes. Pending enrollment state also persists, so a restarted workload resumes its existing request instead of generating a new key and request.

The dashboard has two separate identities. Its container enrolls an app mTLS identity before Express starts, while the browser authenticates directly to the gateway with the owner's WebAuthn passkey.

The governance envelope endpoint rejects app certificates. The unified compose therefore mounts the operator data volume read-only into the ensemble and supplies the operator certificate and key paths through `G8E_GOVERNANCE_OPERATOR_CERT` and `G8E_GOVERNANCE_OPERATOR_KEY`. The ensemble uses that identity only for governed envelope transport; its other gateway traffic uses its own app identity. Approving the operator first ensures these credentials exist before governed requests begin.

See [Authentication and Identity](../architecture/auth.md) for the enrollment protocol and trust model.

## Headless Gateway-Only Deployment

Start only the default profile and enroll a CLI-only owner identity without opening a browser:

```bash
docker compose up -d --build
./g8e auth enroll user --headless -e localhost
```

Headless enrollment skips passkey registration and OS trust installation. The resulting mTLS identity can approve workload enrollment requests from the CLI but cannot sign in to the gateway console. On an already bootstrapped gateway, headless recovery requires approval from another enrolled CLI identity.

## Troubleshooting

### A workload remains unhealthy

List pending requests and inspect the affected service logs:

```bash
./g8e auth pending-platform-enrollments
./g8e docker logs <service>
```

Approve the request, then run `./g8e docker status`. Use Compose service names such as `g8e-operator`, `ensemble`, or `dashboard` with the logs command.

### The automated walkthrough reports no pending request

The walkthrough does not wait for each workload request. List pending requests again after the container has had time to submit, then approve the exact request ID manually.

### Browser authentication or TLS fails

Confirm that the browser uses the same hostname configured by `G8E_HOSTNAME`, that the gateway root CA is trusted by the host, and that the HTTPS port is reachable. Re-run interactive owner enrollment if passkey registration did not complete.

### A fresh gateway conflicts with old local credentials

If the Docker volumes were removed but host-side CLI credentials remain, run `./g8e auth enroll user` against the fresh gateway. The coordinator detects that the gateway has no owner and performs initial bootstrap.

## Relationship to Demo Stacks

The unified stack is separate from the Healthcare, Finance, DHS, and FedRAMP compose projects under `demos/`. Those deployments use organization-specific network segmentation and scenarios and are managed through `./g8e demos`. They do not include the ensemble or dashboard. See the [Demos README](../../demos/README.md).

## Stopping and Removing the Stack

Stop containers while keeping named-volume state:

```bash
docker compose --profile bootstrapped down
# or
./g8e docker stop
```

Remove containers and named volumes only when you intend to destroy the trust domain and all persisted platform state:

```bash
docker compose --profile bootstrapped down -v
# or, without a confirmation prompt by default
./g8e docker clean
```

After volume removal, the next gateway startup creates a new PKI. Enroll the first owner again and approve new workload requests.

## Related Documentation

- [Platform Architecture Overview](../architecture/overview.md): Component roles and the five-layer governance pipeline.
- [Gateway Architecture](../architecture/gateway.md): Gateway services, protocol surfaces, and policy enforcement.
- [Operator Architecture](../architecture/operator.md): Operator execution boundary and L4-L5 lifecycle.
- [Ensemble Architecture](../architecture/ensemble.md): Ensemble role, agent models, and event flow.
- [Dashboard Architecture](../architecture/dashboard.md): Browser and container boundaries.
- [g8ee Documentation](../ensemble/index.md): Ensemble configuration, agents, providers, storage, and testing.
- [g8ed Documentation](../dashboard/index.md): Dashboard authentication, gateway integration, and development.
- [Docker Gateway Guide](./docker_gateway.md): Standalone gateway image operation.
- [Authentication and Identity](../architecture/auth.md): mTLS, WebAuthn, PKI, and platform enrollment.
