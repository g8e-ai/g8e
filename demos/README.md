# g8e Demo Environments

This directory contains Docker Compose demo environments for org-specific g8e deployments. Each org environment is hermetically sealed with no shared state, volumes, or cross-org dependencies.

## Repository Layout

```
demos/
├── bin/                        # built g8e binary, used by g8e demos CLI commands
│   └── g8e
├── images.json                 # Pinned sha256 digests for all external Docker images (air-gapped manifest)
├── healthcare/                # Healthcare/PHI demo
│   ├── compose.yml             # builds from repo-root Dockerfile (context: ../..)
│   ├── config/
│   ├── doctrine/               # PHI/HIPAA scrub patterns
│   ├── target-data/            # Simulated EHR/PA records (narrative reference)
│   ├── README.md               # Healthcare-specific documentation
│   ├── init.sql                # PostgreSQL schema for reporting-db
│   ├── paop.sh                 # PA operation wrapper (governed run_shell_command bridge)
│   └── setup_metabase.py       # Metabase compliance dashboard setup script
├── finance/                    # Finance/trading demo
│   ├── compose.yml             # builds from repo-root Dockerfile (context: ../..)
│   ├── config/
│   ├── doctrine/               # Trading controls and dual-control triggers
│   ├── target-data/            # Simulated ledger/positions
│   └── README.md               # Finance-specific documentation
├── dhs/                        # DHS persistent sovereign capability demo
│   ├── compose.yml             # builds from repo-root Dockerfile (context: ../..)
│   ├── config/                 # Gateway/operator config, consensus-bootstrap.json, ensemble-seed.hex
│   ├── doctrine/               # Sovereign data-handling L1 rules (USPER PII, cross-domain release, destruction)
│   ├── target-data/            # Mock multi-source coalition feeds + sovereign manifest
│   ├── dataop.sh               # Wrapper script bridging operator execution to datasvc
│   ├── datasvc.py              # Sovereign Data Service (L5 actuator, Python HTTP server)
│   ├── verify_ops.py           # Verifies datasvc recorded governed operations
│   └── README.md               # DHS-specific documentation and LOE mapping
├── fedramp/                    # FedRAMP sovereign cloud governance demo
│   ├── compose.yml             # builds from repo-root Dockerfile (context: ../..)
│   ├── config/                 # Gateway/operator config, consensus-bootstrap.json, ensemble-seed.hex
│   ├── doctrine/               # FedRAMP L1 rules (CR-26, AC-2, SI-4, SC-8, CM-7)
│   ├── target-data/            # Cloud resources and KSI control categories
│   ├── cloudop.sh              # Wrapper script bridging operator execution to cloudsvc
│   ├── cloudsvc.py             # Sovereign Cloud Service (L5 actuator, Python HTTP server)
│   ├── verify_ops.py           # Verifies cloudsvc recorded governed operations
│   └── README.md               # FedRAMP-specific documentation
```

## Two Deployment Modes

g8e ships two Docker Compose deployment modes that serve different purposes and must not be confused:

- **Unified platform compose** — the repo-root `docker-compose.yml`. Brings up the whole platform end to end on a single `g8e-net` bridge network: gateway, operator, ensemble (g8ee), and dashboard (g8ed). Run with `docker compose up` from the repo root. This is the default way to run the complete product and is what the v2.0.0 reunification targets. See the [Unified Docker Stack guide](../docs/guides/unified_stack.md).
- **Per-demo composes** — `demos/<org>/compose.yml`. Each demo is an org-specific, hermetically sealed deployment on five isolated networks (untrusted, perimeter, internal, secure, mgmt) that exercises a particular compliance scenario. They build the gateway/operator image from the repo-root `Dockerfile` via `context: ../..` and are driven by the `g8e demos` CLI. The per-demo composes do not include the ensemble or dashboard; they are a separate deployment mode focused on org-specific isolated-network scenarios.

The two modes share the repo-root `Dockerfile` (the Go gateway/operator image) but are otherwise independent: the unified compose adds the ensemble and dashboard services and uses a single flat network, while the per-demo composes use isolated multi-network topologies and are scoped to a single org.

## Network Topology

Each org deploys five isolated networks:

- **net_untrusted**: External/internet simulation. Bad actor services live here.
- **net_perimeter**: DMZ equivalent. Gateway public surface and demo UI.
- **net_internal**: Trusted application tier. AI agents, LLM backend, workflow orchestrators.
- **net_secure**: Privileged tier. Operator and target system. No direct route from net_internal. In the `dhs` and `fedramp` demos, the Gateway is also attached to net_secure so it can reach the actuator boundary.
- **net_mgmt**: Out-of-band observability. Log aggregator and audit tail viewer.

## Service Placement

| Service class | untrusted | perimeter | internal | secure | mgmt |
|---|:---:|:---:|:---:|:---:|:---:|
| External requestor / bad actor sim | ✓ | | | | |
| Demo UI | | ✓ | | | |
| Gateway | | ✓ | ✓ | ✓† | |
| AI agent runtime | | | ✓ | | |
| Operator | | | ✓* | ✓ | |
| Target system | | | | ✓ | |
| Observability stack | | | | | ✓ |

\* Operator appears on net_internal only for its outbound mTLS tunnel to the Gateway. It accepts no inbound connections from net_internal.

\† Gateway appears on net_secure only in the `dhs` and `fedramp` demos, where it needs direct access to the actuator boundary.

The `healthcare` demo runs a typed policy actuator and reporting database on `net_secure`, a Metabase compliance dashboard on `net_perimeter` with a `net_secure` leg to the reporting database, and a one-shot Metabase setup service on `net_perimeter`. Healthcare scenarios use native gateway tools (`run_shell_command` driving the `paop.sh` wrapper) governed by the PHI/HIPAA doctrine engine. The actuator evaluates gold-card and SLA inputs only after admission and records terminal observations bound to the exact run and canonical scenario; no downstream MCP server is involved.

The `dhs` demo deploys a real `agent-coalition` container (the exec target for `g8e demos run`) on net_internal, a real `datasvc` Python HTTP actuator on net_secure, display-only source connectors on net_internal and net_untrusted, and a partner fusion-COP plus a severable coalition datalink on net_perimeter, modeling NIPR/SIPR/Mission-Partner/partner-nation sovereignty boundaries. The `agent-coalition` container is a real g8e binary that submits genuine `GovernanceEnvelope`s when driven by the CLI. The display connectors are Alpine echo loops for narrative only.

The `fedramp` demo deploys a real `agent-runtime` container (the exec target for `g8e demos run`) on net_internal, a real `cloudsvc` Python HTTP actuator on net_secure, a `bad-actor` on net_untrusted, and an `observability` container on net_mgmt. The `agent-runtime` container is a real g8e binary that submits genuine `GovernanceEnvelope`s driving cloud resource operations (provision, configure, destroy, revert) through the `cloudop.sh` wrapper when driven by the CLI.

## Org Differentiation

Each org demonstrates different compliance requirements and use cases:

| Dimension | Healthcare | Finance | DHS | FedRAMP |
|---|---|---|---|---|
| Doctrine content | PHI scrub patterns, PA workflow gates | Tx limits, dual-control triggers | USPER PII minimization, cross-domain release, sovereign destruction | CR-26 audit integrity, AC-2 access control, SI-4 privilege escalation, SC-8 cross-domain, CM-7 config management |
| Target data content | Simulated EHR / PA records | Simulated ledger / positions | Mock multi-source coalition feeds + sovereign manifest | Cloud resources (VMs, DBs, IAM) and KSI control categories |
| Gateway posture | doctrine | doctrine | consensus | consensus |
| Agent principal type | Clinical AI agent | Algorithmic trading agent | Coalition source connectors + predictive-analytics agent | FedRAMP cloud service operator |
| Target system mock | EHR / PA processor | Trade execution API | Sovereign data vault + partner fusion COP | Sovereign cloud service (provision, configure, destroy, revert) |
| Demo narrative | PHI scrub + PA approval flow | Unauthorized trade blocked | Sovereign coalition data plane: govern ingest, release, use, destruction | Sovereign cloud governance: provision, destroy, revert, audit integrity |

## Quick Start

### Prerequisites

- Docker and Docker Compose installed
- g8e binary built and copied to `demos/bin/`

### Build the g8e binary

From the repository root:

```bash
make build
```

`make build` automatically copies the binary to `demos/bin/g8e`. Demo compose files build the g8e container image from the repo-root `Dockerfile` (via `context: ../..`), which compiles the binary inside the container with FIPS 140-3 approved mode enabled (`GOFIPS140=v1.0.0`). The `demos/bin/g8e` binary is not used by the container image — it is consumed only by `g8e demos` CLI commands that run on the host.

### Using the g8e CLI (recommended)

The g8e CLI provides convenient commands for managing demo environments:

```bash
# List available demo environments
g8e demos list

# Start a demo environment
g8e demos start <org>

# Check service status
g8e demos status <org>

# Stop a demo environment
g8e demos stop <org>

# Clean a demo environment (remove containers, volumes, and networks)
# Confirmation is skipped by default; use --yes=false to prompt
g8e demos clean <org>

# Clean all demo environments
g8e demos clean

# Rebuild Docker images and restart a demo environment
# Uses --no-cache by default; pass --no-cache=false to reuse cache
g8e demos rebuild <org>

# Reset a demo environment (clean and restart)
g8e demos reset <org>

# Run a specific demo scenario (concise output by default)
g8e demos run <org> <scenario>

# Run with verbose step-by-step output
g8e demos run <org> <scenario> -v

# Run with the tactical governance TUI overlay
g8e demos run <org> <scenario> --tui

# Pre-pull all external images for air-gapped deployment
g8e demos pull
```

`g8e demos run <org>` prints every scenario result, then exits non-zero with `ErrDemoScenarioFailed` if any result is `FAIL`. Automation can gate on the exit status, but evidence capture still retains and reviews the complete result table, including skipped and failed rows.

### Owner-approved platform bootstrap

Every demo boots the gateway with zero users. The operator (and any service that depends on it) starts not-ready and remains not-ready until its owner-approved platform enrollment request is approved. After `g8e demos start <org>` completes, the CLI prints the bootstrap instructions: enroll the first owner, list pending platform enrollment requests, and approve the operator's request by exact request ID.

```bash
# 1. Enroll the first owner (the demo gateway port is printed by `g8e demos start <org>`).
./g8e auth enroll user -e https://localhost:<demo-https-port>

# 2. List pending platform enrollment requests.
./g8e auth pending-platform-enrollments

# 3. Approve the operator's request by exact request ID.
./g8e auth approve-platform-enrollment <operator-request-id> --yes

# 4. Wait for the operator and its dependents to become healthy.
g8e demos status <org>
```

`g8e demos run <org>` warns if the operator is not yet enrolled and prints the bootstrap instructions before attempting to run scenarios. Do not use `docker compose up --wait` before approval; it is expected to time out while enrollment is pending. See the [Docker Gateway Guide](../docs/guides/docker_gateway.md) for the full bootstrap flow and the headless deployment mode.

### Audit Commands

Audit commands are top-level, not nested under `demos`. They query the running Gateway's audit store:

```bash
# List signed receipts
g8e audit receipts

# Query raw audit events
g8e audit events

# Aggregate event and receipt counts by type
g8e audit summary

# Export the full receipts bundle for archival
g8e audit export

# Generate a compliance report (JSON)
g8e audit report
```

### Demo Scenarios

Each demo environment includes predefined scenarios that demonstrate specific security and compliance features:

**Healthcare Demo Scenarios:**
- `g8e demos run healthcare 1` - Authorized Agent Submits a FHIR PA Request
- `g8e demos run healthcare 2` - Gold Card Auto-Approval
- `g8e demos run healthcare 3` - SLA Breach and OHA Reporting
- `g8e demos run healthcare 4` - Bad Actor PHI Exfiltration Blocked

**Finance Demo Scenarios:**
- `g8e demos run finance 1` - Unauthorized Trade Blocked

**DHS Persistent Sovereign Capability Demo Scenarios:**
- `g8e demos run dhs 1` - Sovereign Multi-Source Ingest (chain-of-custody) (LOE 1)
- `g8e demos run dhs 2` - Resilient Disconnected Operations / Continuity of Coverage (LOE 2)
- `g8e demos run dhs 3` - Governed Predictive Cueing (LOE 3 & 4)
- `g8e demos run dhs 4` - Sovereign Destruction + tamper-evident audit (LOE 2)

**FedRAMP Sovereign Cloud Governance Demo Scenarios:**
- `g8e demos run fedramp 1` - Governed Cloud Resource Provisioning
- `g8e demos run fedramp 2` - Unauthorized Audit Trail Destruction Blocked (CR-26)
- `g8e demos run fedramp 3` - Governed Configuration Revert (CM-7)
- `g8e demos run fedramp 4` - Gateway Audit Vault Destruction Blocked (CR-26)

### Ensemble Demo Scenarios

Ensemble scenarios exercise the core product path: a user sends a chat message to the ensemble (g8ee), the AI reasons about the request and submits governed tool calls through the operator to the gateway for admission. The gateway runs the 5-layer governance gauntlet (L1 doctrine, L2 consensus, L3 notary, L4 warden, L5 actuator), and the operator executes the admitted command. The scenarios verify the end-to-end flow by polling the audit vault for signed ActionReceipts.

Ensemble scenarios require the **unified platform compose** (`docker-compose.yml` at the repo root), not the per-demo composes (which do not include the ensemble). Bring up the full stack with the two-phase bootstrap profile:

```bash
# 1. Start the gateway (default profile).
docker compose up -d g8e-gateway

# 2. Enroll the first owner.
./g8e auth enroll user -e https://localhost:8443

# 3. Bring up the bootstrapped workloads (operator, ensemble, dashboard).
docker compose --profile bootstrapped up -d

# 4. Approve the workload enrollment requests.
./g8e auth pending-platform-enrollments
./g8e auth approve-platform-enrollment <operator-request-id> --yes
./g8e auth approve-platform-enrollment <ensemble-request-id> --yes
./g8e auth approve-platform-enrollment <dashboard-request-id> --yes

# 5. Refresh the owner CLI session after Operator enrollment.
./g8e auth refresh
```

The initial owner CLI session predates the Operator session. Refreshing it after workload enrollment binds the canonical CLI session to the active Operator session and prevents downstream identity validation from failing closed with a session-binding mismatch.

Run ensemble scenarios via the agent harness:

```bash
g8e demos scenarios run ensemble-chat-file-create \
  --mtls-url https://localhost:8443 \
  --public-url http://localhost:8080 \
  --ensemble-url http://localhost:8000 \
  --user-id <user-id> \
  --cli-session-id <cli-session-id>
```

The `--ensemble-url` flag points the harness at the ensemble (g8ee) HTTP surface. Ensemble scenarios use the `GovKit` pattern for identity binding (operator session, user ID, CLI session ID) and poll the audit vault for signed receipts.

**Available ensemble scenarios:**

- `ensemble-chat-file-create` - AI creates a governed file via the `file_create` tool. Verifies a `FILE_EDIT` receipt with `COMPLETED` status appears in the audit vault.
- `ensemble-chat-file-write` - AI writes content to an existing file via the `file_write` tool. Verifies a `FILE_EDIT` receipt.
- `ensemble-document-update` - AI triggers a case/investigation create via the ensemble. Verifies a `DOCUMENT_UPDATE` envelope was admitted by L1 and persisted via the document store handler.
- `ensemble-document-delete` - AI triggers a document delete. Verifies a `DOCUMENT_DELETE` envelope was admitted and the document was removed.

**LLM provider selection:**

Ensemble scenarios default to the `fake` LLM provider for CI determinism (no external LLM dependency). The `FakeProvider` (`ensemble/app/llm/providers/fake.py`) returns deterministic tool-call responses by pattern-matching the user message — no network calls, no API keys required.

For local dev with a real LLM (e.g., ollama), set env vars before running the harness:

```bash
export G8E_HARNESS_LLM_PROVIDER=ollama
export G8E_HARNESS_LLM_MODEL=gemma4:12b
export G8E_HARNESS_LLM_ENDPOINT=http://192.168.1.2:11434
g8e demos scenarios run ensemble-chat-file-create --ensemble-url http://localhost:8000 ...
```

### Canonical scenario definitions

Every demo scenario has a typed, content-addressed definition in the canonical demo scenario catalog at `protocol/constants/compliance/demo-scenario-catalog.json`. Each definition carries stable scenario identity and version, expected action classes, expected allow or block outcome, expected rejection layer, initial-state fixture reference, terminal-state assertions, required receipts and deterministic stages, assertion references, framework-control references, required evidence level, timeout, and failure policy. The catalog is the source of truth for scenario identity, assertion mappings, and framework-control mappings. The prose descriptions in the per-demo READMEs and the scenario listings below are narrative context; the canonical catalog is authoritative for all machine-checked compliance crosswalks and evidence-grade result production.

The catalog is loaded at demo runtime through `internal/services/compliance/catalog` and validated against the canonical assertion catalog (`protocol/constants/compliance/assertion-catalog.json`) and framework catalog (`protocol/constants/compliance/framework-catalog.json`). Every framework-control reference in every definition has at least one canonical crosswalk mapping (`protocol/constants/compliance/fedramp-nist-crosswalk.json`) to an assertion carried by that definition.

### Demo Output Format

By default, `g8e demos run` produces concise output: each scenario prints its PASS/FAIL result line. After all scenarios complete, a results table summarizes scenario numbers, names, statuses, and key metrics.

Use `-v` (or `--verbose`) to see full step-by-step command output, scenario descriptions, and `PROVES:` annotations. Use `--tui` to launch the tactical governance TUI overlay with live pipeline and consensus visualization.

Note: The `g8e demos run` command automatically starts the demo environment if it is not already running.

### Manual Docker Compose commands

You can also use Docker Compose directly:

```bash
cd demos/finance
docker compose up -d
```

### Check service status

```bash
docker compose ps
docker compose logs gateway
docker compose logs operator
```

### Stop an org environment

```bash
docker compose down
```

### Clean up all runtime state

```bash
docker compose down -v
```

## Standing Up a New Org

To create a new org environment:

```bash
cp -r demos/healthcare demos/neworg
```

Then:
- Replace `./neworg/doctrine/` with org-specific L1 rules
- Replace `./neworg/target-data/` with org-specific simulated assets
- Edit `./neworg/config/gateway.yml` and `operator.yml` for org identity values
- Edit `compose.yml` service labels and container names for the new org

```bash
cd demos/neworg && docker compose up
```

Demo compose files build from the repo-root `Dockerfile` via `context: ../..`. There is no demo-specific Dockerfile; demos use the same production image that ships to deployment. Run `make build` first to produce the host-side `demos/bin/g8e` binary used by `g8e demos` CLI commands (the container image builds its own binary from source).

## Invariants

The following must hold in every org environment:

1. The Operator is the only g8e process on net_secure. No agent, no observability service is co-located on net_secure. In the `dhs` and `fedramp` demos, the Gateway is also attached to net_secure so it can reach the actuator boundary.
2. No named volume is shared with write access between services. Each service owns its own writable volume; read-only mounts are permitted for agent credential sharing (`operator_state:ro` on agent-runtime/agent-coalition) and observability audit tailing (`gateway_state:ro`, `operator_state:ro`).
3. No PKI material is pre-distributed via filesystem. Identity propagates through enrollment over mTLS.
4. Doctrine is a bind mount, not baked into an image. Org behavior is data, not code.
5. The repo-root `Dockerfile` is the only build artifact shared across org directories. Each compose file references `build: context: ../..` to build the production image (with FIPS 140-3 approved mode) from the repo root. All agent containers (`agent-runtime` / `agent-coalition`) use the same image with `entrypoint: ["sh", "-c", "sleep infinity"]` for exec-based scenarios run invocation.

## Port Mappings

Each org uses different host ports to allow simultaneous deployment:

| Org | HTTP Port | HTTPS Port | Additional Ports |
|---|---|---|---|
| healthcare | 8081 | 8444 | Metabase: 3001, PostgreSQL: 5433 |
| finance | 8082 | 8445 | Demo UI: 3002 |
| dhs | 8087 | 8450 | |
| fedramp | 8088 | 8451 | FIPS 140-3 approved mode is the default for all orgs (single Dockerfile) |

All demo images build with FIPS 140-3 approved mode enabled (`GOFIPS140=v1.0.0` in the Dockerfile builder stage). Enforcement is off by default; set `GODEBUG=fips140=only` in a service's `environment:` block to reject non-approved primitives at runtime. See [FIPS 140-3 Compliance](../docs/reference/fips140-3.md) for details.

## PKI and Enrollment

There is no shared PKI volume. There is no init container that pre-populates certificates for downstream services.

**Sequence:**

1. Gateway container starts, generates its own CA, initializes PKI under its named volume, begins listening for enrollment on the mTLS port
2. Operator container starts, reads its config (gateway endpoint + device-link token), dials the Gateway over mTLS, completes enrollment, receives its identity certificate
3. From that point forward the Operator holds its own identity material in its own named volume

`depends_on` with `condition: service_healthy` enforces ordering. The Gateway health check (`/api/v1/health`) must pass before the Operator starts.

This reflects real deployment: the Operator on a remote air-gapped host has no filesystem access to the Gateway.

## Observability

The observability container mounts audit vault and state volumes read-only, providing out-of-band inspection of the audit trail and actuator state without requiring live network connections to services. In the `healthcare`, `finance`, and `dhs` demos it actively tails the gateway platform log (`/data/gateway/logs/g8e.log`); the `fedramp` observability container exposes the mounted volumes for manual `docker compose exec` inspection.

## Troubleshooting

### Gateway health check failing

Check Gateway logs:
```bash
docker compose logs gateway
```

Verify the Gateway is listening on the expected ports:
```bash
docker compose exec gateway netstat -tlnp
```

### Operator enrollment failing

Check Operator logs:
```bash
docker compose logs operator
```

Verify the Operator can reach the Gateway:
```bash
docker compose exec operator ping gateway
```

### Doctrine not loading

Verify doctrine files are present and valid JSON:
```bash
docker compose exec gateway ls -la /etc/g8e/doctrine/
docker compose exec gateway cat /etc/g8e/doctrine/*.json
```

### Target data not accessible

Verify target-data mount:
```bash
docker compose exec target-system ls -la /var/g8e/target/
```

## Air-Gapped Deployment

g8e demos can be deployed in environments with no network access. All Go
dependencies are vendored, all Docker images are pinned to sha256 digests, and
no runtime `pip install` or external package fetches are required.

### Prerequisites

- Docker and Docker Compose installed on both the connected and air-gapped machines
- The `g8e` binary built from source (or use `make build`)

### Step 1: Pre-pull Images (Connected Machine)

On a machine with internet access, pull all external images listed in the
manifest:

```bash
g8e demos pull
```

This reads `demos/images.json` and pulls each image by its pinned digest.

### Step 2: Export Images (Connected Machine)

Save the pulled images to tar files for transfer:

```bash
g8e demos export /tmp/g8e-images
```

This creates one `.tar` file per image in the specified output directory.

### Step 3: Transfer to Air-Gapped Machine

Copy the entire repository (including the `vendor/` directory) and the exported
image directory to the air-gapped machine via your approved transfer mechanism
(e.g., secure USB, DLP-approved file transfer).

### Step 4: Import Images (Air-Gapped Machine)

Load the exported images into the local Docker daemon:

```bash
g8e demos import /tmp/g8e-images
```

### Step 5: Build and Run (Air-Gapped Machine)

The build uses vendored Go modules and does not require network access. The repo-root Dockerfile compiles the binary inside the container from vendored sources:

```bash
make build
g8e demos start <org>
```

### Verification

Run the air-gap verification target to confirm everything is in order:

```bash
make test-airgap
```

This checks that:
- `vendor/` directory exists
- The vendored build compiles without network access
- `demos/images.json` manifest is present
- No unpinned image references remain in compose files
- No `pip install` or `import requests` references remain in demo Python files

### Image Manifest

The `demos/images.json` file lists all external Docker images used across all
demo environments, along with their pinned sha256 digests and which demos use
each image. To list all images in the manifest:

```bash
g8e demos images
```

## License

Business Source License 1.1 (BSL 1.1). Converts to Apache 2.0 on 2030-08-18.
