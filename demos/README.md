# g8e Demo Environments

This directory contains Docker Compose demo environments for org-specific g8e deployments. Each org environment is hermetically sealed with no shared state, volumes, or cross-org dependencies.

## Repository Layout

```
demos/
├── bin/                        # built g8e binary, used by g8e demos CLI commands
│   └── g8e
├── gov/                        # Government/CUI demo
│   ├── compose.yml
│   ├── config/
│   │   ├── gateway.yml
│   │   └── operator.yml
│   ├── doctrine/               # CUI/CMMC L1 pattern rules
│   ├── target-data/            # Simulated classified document store
│   └── README.md               # Gov-specific documentation
├── healthcare/                # Healthcare/PHI demo
│   ├── compose.yml
│   ├── config/
│   ├── doctrine/               # PHI/HIPAA scrub patterns
│   ├── target-data/            # Simulated EHR/PA records
│   ├── README.md               # Healthcare-specific documentation
│   ├── init.sql                # PostgreSQL schema for reporting-db
│   ├── pa_api_server.py        # FHIR R4 PA submission API server
│   └── setup_metabase.py       # Metabase compliance dashboard setup script
├── finance/                    # Finance/trading demo
│   ├── compose.yml
│   ├── config/
│   ├── doctrine/               # Trading controls and dual-control triggers
│   ├── target-data/            # Simulated ledger/positions
│   └── README.md               # Finance-specific documentation
├── secure-data/                # Governed data migration / two-operator pipeline demo
│   ├── compose.yml
│   ├── config/
│   ├── doctrine/               # Migration-screening L1 rules (bypass, exfil, cross-tenant)
│   ├── target-data/            # Simulated SharePoint migration manifest
│   └── README.md               # Secure-data-specific documentation
├── dow/                        # Department of War tactical edge demo
│   ├── compose.yml
│   ├── config/
│   ├── doctrine/               # Tactical edge L1 rules (spoofing, cross-cue, EW, weapons)
│   ├── target-data/            # Simulated RF environment, PNT sources, payload manifest
│   ├── dow_simulator.py        # Display-only sensor narration (EO/IR, PNT fusion)
│   ├── gimbal.py               # Mock gimbal HTTP server (records slew commands)
│   ├── slew.sh                 # Demo artifact: wraps gimbal HTTP call for run_shell_command
│   ├── inspect_rf.py           # RF environment inspection helper for sensor agents
│   ├── inspect_pnt.py          # PNT source inspection helper for sensor agents
│   ├── verify_slews.py         # Verifies gimbal slew commands were recorded
│   └── README.md               # DoW-specific documentation
├── dhs/                        # DHS persistent sovereign capability demo
│   ├── compose.yml
│   ├── config/                 # Gateway/operator config, tribunal-bootstrap.json, ensemble-seed.hex
│   ├── doctrine/               # Sovereign data-handling L1 rules (USPER PII, cross-domain release, destruction)
│   ├── target-data/            # Mock multi-source coalition feeds + sovereign manifest
│   ├── dataop.sh               # Wrapper script bridging operator execution to datasvc
│   ├── datasvc.py              # Sovereign Data Service (L5 actuator, Python HTTP server)
│   ├── verify_ops.py           # Verifies datasvc recorded governed operations
│   └── README.md               # DHS-specific documentation and LOE mapping
├── swarm/                      # Drone swarm battlefield demo
│   ├── compose.yml
│   ├── config/
│   ├── doctrine/               # Drone operations doctrine (weapons, safety, navigation)
│   ├── target-data/            # Battlefield intelligence and fleet manifest
│   ├── drone_simulator.py      # Per-drone telemetry simulation script
│   └── README.md               # Swarm-specific documentation
├── frontend/                   # Frontend enrollment demo
│   ├── compose.yml
│   ├── config/
│   ├── doctrine/               # Frontend security rules (API access, CORS spoofing, session hijacking)
│   ├── app/                    # Single-file HTML frontend app served by nginx
│   └── README.md               # Frontend-specific documentation
└── live-swarm/                 # Live swarm demo (manual, no Docker Compose)
    ├── drone_cmd.py            # MAVSDK bridge script for PX4 SITL
    ├── tribunal-bootstrap.json # Tribunal seed template for notary posture
    └── README.md               # Walkthrough guide for live recording
```

## Network Topology

Each org deploys five isolated networks:

- **net_untrusted**: External/internet simulation. Bad actor services live here.
- **net_perimeter**: DMZ equivalent. Gateway public surface and demo UI.
- **net_internal**: Trusted application tier. AI agents, LLM backend, workflow orchestrators.
- **net_secure**: Privileged tier. Operator and target system. No direct route from net_internal. In the `dow` and `dhs` demos, the Gateway is also attached to net_secure so it can reach the actuator boundary.
- **net_mgmt**: Out-of-band observability. Log aggregator and audit tail viewer.

The `secure-data` demo replaces the standard five-network topology with a two-domain layout: **net_src_internal** (source gateway, operator, connectors, source storage), **net_dst_internal** (destination gateway, operator, destination storage), **net_migration** (transfer bridge between source and destination operators), **net_untrusted** (bad actor), and **net_mgmt** (observability).

## Service Placement

| Service class | untrusted | perimeter | internal | secure | mgmt |
|---|:---:|:---:|:---:|:---:|:---:|
| External requestor / bad actor sim | ✓ | | | | |
| Demo UI / Notary approval UI | | ✓ | | | |
| Gateway | | ✓ | ✓ | ✓† | |
| AI agent runtime | | | ✓ | | |
| Operator | | | ✓* | ✓ | |
| Target system | | | | ✓ | |
| Observability stack | | | | | ✓ |

\* Operator appears on net_internal only for its outbound mTLS tunnel to the Gateway. It accepts no inbound connections from net_internal.

\† Gateway appears on net_secure only in the `dow` and `dhs` demos, where it needs direct access to the actuator boundary.

The `healthcare` demo adds PA workflow services on net_secure (pa-submission-service, provider-exemption-rules, pa-processing-worker, message-broker, reporting-db) and a Metabase compliance dashboard on net_perimeter. The `swarm` demo deploys 20 operator containers (8 recon, 6 attack, 4 support, 2 relay) plus a command interface on net_internal. The `secure-data` demo deploys two gateway-operator pairs (source and destination) with connectors on net_src_internal.

The `dow` demo deploys three sensor agent containers (SIGINT, EO/IR, PNT fusion) on net_internal, a simulated ground station on net_perimeter, an EW adversary on net_untrusted, and a mock gimbal controller on net_secure, with SWaP resource limits on all g8e containers. The `agent-sigint` container is a real g8e binary that submits genuine GovernanceEnvelopes. `agent-eoir` and `agent-pnt-fusion` use `dow_simulator.py` for display-only narration.

The `dhs` demo deploys a real `agent-coalition` container (running `demos scenarios run`) on net_internal, a real `datasvc` Python HTTP actuator on net_secure, display-only source connectors on net_internal and net_untrusted, and a partner fusion-COP plus a severable coalition datalink on net_perimeter, modeling NIPR/SIPR/Mission-Partner/partner-nation sovereignty boundaries. The `agent-coalition` container is a real g8e binary that submits genuine `GovernanceEnvelope`s. The display connectors are Alpine echo loops for narrative only.

The `frontend` demo deploys a nginx-served static HTML app on net_perimeter for WebAuthn passkey enrollment and SSE event streaming. No target system or bad actor container. The gateway runs in doctrine posture with CORS and passkey RP origins pre-configured for the frontend origin.

## Org Differentiation

Each org demonstrates different compliance requirements and use cases:

| Dimension | Gov | Healthcare | Finance | Secure-Data | DoW | Swarm | DHS | Frontend |
|---|---|---|---|---|---|---|---|---|
| Doctrine content | CUI, classification markings, CMMC rules | PHI scrub patterns, PA workflow gates | Tx limits, dual-control triggers | Migration-screening rules (bypass, exfil, cross-tenant) | GPS spoofing, cross-cue, EW, weapons control, PNT BFT | Weapons control, safety, navigation, command integrity | USPER PII minimization, cross-domain release, sovereign destruction | Unauthorized API access, CORS origin spoofing, session hijacking |
| Target data content | Simulated classified document store | Simulated EHR / PA records | Simulated ledger / positions | Simulated SharePoint migration manifest | RF environment, PNT sources (incl. spoofed), payload manifest | Battlefield intelligence and drone fleet manifest | Mock multi-source coalition feeds + sovereign manifest | None (frontend enrollment demo) |
| Gateway posture | doctrine | consensus | doctrine | doctrine | doctrine | doctrine | consensus | doctrine |
| Agent principal type | DoD contractor agent | Clinical AI agent | Algorithmic trading agent | Data migration connector (rclone, SharePoint) | Tactical sensor agent (SIGINT, EO/IR, PNT fusion) | Autonomous drone operator (recon, attack, support, relay) | Coalition source connectors + predictive-analytics agent | Browser-based frontend app (WebAuthn + SSE) |
| Target system mock | Classified doc API | EHR / PA processor | Trade execution API | Source and destination storage endpoints | Gimbal/flight controller actuator | Drone fleet with telemetry simulation | Sovereign data vault + partner fusion COP | None (nginx-served static HTML) |
| Demo narrative | CUI exfil attempt blocked | PHI scrub + PA approval flow | Unauthorized trade blocked | Governed migration with chain-of-custody receipts | SIGINT cross-cue + BFT spoofing defense + disconnected ops | Adversary interception and safety violation detection | Sovereign coalition data plane: govern ingest, release, use, destruction | Third-party frontend enrollment with CORS and passkey authentication |

## Quick Start

### Prerequisites

- Docker and Docker Compose installed
- g8e binary built and copied to `demos/bin/`

### Build the g8e binary

From the repository root:

```bash
make build
```

`make build` automatically copies the binary to `demos/bin/g8e`.

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

**Gov Demo Scenarios:**
- `g8e demos run gov 1` - CUI Exfiltration Attempt Blocked

**DoW Demo Scenarios:**
- `g8e demos run dow 1` - Autonomous SIGINT-to-EO/IR Cross-Cueing (Challenge 5)
- `g8e demos run dow 2` - BFT Spoofing Defense (Challenge 8)
- `g8e demos run dow 3` - Disconnected Operations (Challenge 6)

**Finance Demo Scenarios:**
- `g8e demos run finance 1` - Unauthorized Trade Blocked

**Governed Data Migration Demo Scenarios:**
- `g8e demos run secure-data 1` - Governed Migration with Chain-of-Custody Receipts
- `g8e demos run secure-data 2` - Connector Bypass Attempt Blocked
- `g8e demos run secure-data 3` - Cross-Tenant Leak Doctrine Triggered

**DHS Persistent Sovereign Capability Demo Scenarios:**
- `g8e demos run dhs 1` - Sovereign Multi-Source Ingest (chain-of-custody) (LOE 1)
- `g8e demos run dhs 2` - Cross-Domain Release requires Notary authority (LOE 1 & 2)
- `g8e demos run dhs 3` - Resilient Disconnected Operations / Continuity of Coverage (LOE 2)
- `g8e demos run dhs 4` - Governed Predictive Cueing (quorum vs veto) (LOE 3 & 4)
- `g8e demos run dhs 5` - Sovereign Destruction + tamper-proof audit (LOE 2)

**Swarm Demo Scenarios:**
- `g8e demos run swarm 1` - Authorized Recon Mission (Governed Drone Deployment)
- `g8e demos run swarm 2` - Weapons Safety Doctrine Block
- `g8e demos run swarm 3` - Navigation Boundary Violation Block

**Frontend Demo Scenarios:**
- `g8e demos run frontend 1` - Third-Party Frontend Enrollment

### Demo Output Format

By default, `g8e demos run` produces concise output: each scenario prints its PASS/FAIL result line. After all scenarios complete, a results table summarizes scenario numbers, names, statuses, and key metrics.

Use `-v` (or `--verbose`) to see full step-by-step command output, scenario descriptions, and `PROVES:` annotations. Use `--tui` to launch the tactical governance TUI overlay with live pipeline and consensus visualization.

Note: The `g8e demos run` command automatically starts the demo environment if it is not already running.

### Manual Docker Compose commands

You can also use Docker Compose directly:

```bash
cd demos/gov
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

The `demos/Dockerfile` is shared across all org directories. It copies the pre-built binary from `demos/bin/g8e` into a minimal Debian image — no compilation happens inside the container. Run `make build` first to produce the binary.

## Invariants

The following must hold in every org environment:

1. The Operator is the only g8e process on net_secure. No agent, no observability service is co-located on net_secure. In the `dow` and `dhs` demos, the Gateway is also attached to net_secure so it can reach the actuator boundary.
2. No named volume is shared between services. Each service owns its own volume.
3. No PKI material is pre-distributed via filesystem. Identity propagates through enrollment over mTLS.
4. Doctrine is a bind mount, not baked into an image. Org behavior is data, not code.
5. The `demos/Dockerfile` is the only build artifact shared across org directories. Each compose file references `build: context: ..` to copy the pre-built binary from `demos/bin/g8e` into the container. No compilation happens inside the container. All `agent-runtime` containers use the same image with `entrypoint: ["sh", "-c", "sleep infinity"]` for exec-based scenarios run invocation.

## Port Mappings

Each org uses different host ports to allow simultaneous deployment:

| Org | HTTP Port | HTTPS Port | Additional Ports |
|---|---|---|---|
| gov | 8080 | 8443 | Demo UI: 3000 |
| healthcare | 8081 | 8444 | Metabase: 3001, RabbitMQ Mgmt: 15673, PostgreSQL: 5433 |
| finance | 8082 | 8445 | Demo UI: 3002 |
| secure-data (src) | 8083 | 8446 | |
| secure-data (dst) | 8084 | 8447 | |
| dow | 8086 | 8449 | |
| swarm | 8085 | 8448 | Command Interface: 5005 |
| dhs | 8087 | 8450 | |
| frontend | 8083 | 8446 | Frontend App: 3003 |

The `frontend` demo shares ports 8083 and 8446 with the `secure-data` (source) demo. Do not run both simultaneously.

## PKI and Enrollment

There is no shared PKI volume. There is no init container that pre-populates certificates for downstream services.

**Sequence:**

1. Gateway container starts, generates its own CA, initializes PKI under its named volume, begins listening for enrollment on the mTLS port
2. Operator container starts, reads its config (gateway endpoint + device-link token), dials the Gateway over mTLS, completes enrollment, receives its identity certificate
3. From that point forward the Operator holds its own identity material in its own named volume

`depends_on` with `condition: service_healthy` enforces ordering. The Gateway health check (`/api/v1/health`) must pass before the Operator starts.

This reflects real deployment: the Operator on a remote air-gapped host has no filesystem access to the Gateway.

## Observability

The observability container mounts audit vault volumes read-only and tails log files. This provides out-of-band inspection of the audit trail without requiring live network connections to services.

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

The build uses vendored Go modules and does not require network access. The demos Dockerfile copies the pre-built binary into the container without compilation:

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

Apache 2.0
