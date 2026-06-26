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
│   └── target-data/            # Simulated classified document store
├── healthcare/                # Healthcare/PHI demo
│   ├── compose.yml
│   ├── config/
│   ├── doctrine/               # PHI/HIPAA scrub patterns
│   ├── target-data/            # Simulated EHR/PA records
│   ├── healthcare.md           # Demo narrative and scenario walkthrough
│   ├── init.sql                # PostgreSQL schema for reporting-db
│   ├── pa_api_server.py        # FHIR R4 PA submission API server
│   └── setup_metabase.py       # Metabase compliance dashboard setup script
├── finance/                    # Finance/trading demo
│   ├── compose.yml
│   ├── config/
│   ├── doctrine/               # Trading controls and dual-control triggers
│   └── target-data/            # Simulated ledger/positions
├── secure-data/                # Governed data migration / two-operator pipeline demo
│   ├── compose.yml
│   ├── config/
│   ├── doctrine/               # Migration-screening L1 rules (bypass, exfil, cross-tenant)
│   └── target-data/            # Simulated SharePoint migration manifest
├── dow/                        # Department of War tactical edge demo
│   ├── compose.yml
│   ├── config/
│   ├── doctrine/               # Tactical edge L1 rules (spoofing, cross-cue, EW, weapons)
│   ├── target-data/            # Simulated RF environment, PNT sources, payload manifest
│   ├── dow_simulator.py        # Display-only sensor narration (EO/IR, PNT fusion)
│   ├── gimbal.py               # Mock gimbal HTTP server (records slew commands)
│   ├── slew.sh                 # Demo artifact: wraps gimbal HTTP call for run_shell_command
│   └── README.md               # DoW-specific documentation
└── swarm/                      # Drone swarm battlefield demo
    ├── compose.yml
    ├── config/
    ├── doctrine/               # Drone operations doctrine (weapons, safety, navigation)
    ├── target-data/            # Battlefield intelligence and fleet manifest
    ├── drone_simulator.py      # Per-drone telemetry simulation script
    └── README.md               # Swarm-specific documentation
```

## Network Topology

Each org deploys five isolated networks:

- **net_untrusted**: External/internet simulation. Bad actor services live here.
- **net_perimeter**: DMZ equivalent. Gateway public surface and demo UI.
- **net_internal**: Trusted application tier. AI agents, LLM backend, workflow orchestrators.
- **net_secure**: Privileged tier. Operator and target system only. No direct route from net_internal. In the `secure-data` demo, this is split into **net_src_internal**, **net_dst_internal**, and **net_migration**.
- **net_mgmt**: Out-of-band observability. Log aggregator and audit tail viewer.

## Service Placement

| Service class | untrusted | perimeter | internal | secure | mgmt |
|---|:---:|:---:|:---:|:---:|:---:|
| External requestor / bad actor sim | ✓ | | | | |
| Demo UI / Notary approval UI | | ✓ | | | |
| Gateway | | ✓ | ✓ | | |
| AI agent runtime | | | ✓ | | |
| LLM backend | | | ✓ | | |
| Operator | | | ✓* | ✓ | |
| Target system | | | | ✓ | |
| Observability stack | | | | | ✓ |

* Operator appears on net_internal only for its outbound mTLS tunnel to the Gateway. It accepts no inbound connections from net_internal.

The `healthcare` demo adds PA workflow services on net_secure (pa-submission-service, provider-exemption-rules, pa-processing-worker, message-broker, reporting-db) and a Metabase compliance dashboard on net_perimeter. The `swarm` demo deploys 20 operator containers (8 recon, 6 attack, 4 support, 2 relay) plus a command interface on net_internal. The `secure-data` demo deploys two gateway-operator pairs (source and destination) with connectors on net_src_internal. The `dow` demo deploys three sensor agent containers (SIGINT, EO/IR, PNT fusion) on net_internal, a simulated ground station on net_perimeter, an EW adversary on net_untrusted, and a mock gimbal controller on net_secure, with SWaP resource limits on all g8e containers. The `agent-sigint` container is a real g8e binary that submits genuine GovernanceEnvelopes; `agent-eoir` and `agent-pnt-fusion` still use `dow_simulator.py` for display-only narration.

## Org Differentiation

Each org demonstrates different compliance requirements and use cases:

| Dimension | Gov | Healthcare | Finance | Secure-Data | DoW | Swarm |
|---|---|---|---|---|---|---|
| Doctrine content | CUI, classification markings, CMMC rules | PHI scrub patterns, PA workflow gates | Tx limits, dual-control triggers | Migration-screening rules (bypass, exfil, cross-tenant) | GPS spoofing, cross-cue, EW, weapons control, PNT BFT | Weapons control, safety, navigation, command integrity |
| Target data content | Simulated classified document store | Simulated EHR / PA records | Simulated ledger / positions | Simulated SharePoint migration manifest | RF environment, PNT sources (incl. spoofed), payload manifest | Battlefield intelligence and drone fleet manifest |
| Gateway posture | notary | consensus | notary | notary | consensus | consensus |
| Agent principal type | DoD contractor agent | Clinical AI agent | Algorithmic trading agent | Data migration connector (rclone, SharePoint) | Tactical sensor agent (SIGINT, EO/IR, PNT fusion) | Autonomous drone operator (recon, attack, support, relay) |
| Target system mock | Classified doc API | EHR / PA processor | Trade execution API | Source and destination storage endpoints | Gimbal/flight controller actuator | Drone fleet with telemetry simulation |
| Demo narrative | CUI exfil attempt blocked | PHI scrub + PA approval flow | Unauthorized trade blocked | Governed migration with chain-of-custody receipts | SIGINT cross-cue + BFT spoofing defense + disconnected ops | Adversary interception and safety violation detection |

## Quick Start

### Prerequisites

- Docker and Docker Compose installed
- g8e binary built and copied to `demos/bin/`

### Build the g8e binary

From the repository root:

```bash
make build
cp g8e demos/bin/g8e
```

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
g8e demos clean <org>

# Reset a demo environment (clean and restart)
g8e demos reset <org>

# Run a specific demo scenario
g8e demos run <org> <scenario>

# View audit logs and ledger history for a running demo
g8e demos audit <org>

# Tail the observability log stream
g8e demos audit <org> logs

# Open the gateway audit database (SQLite)
g8e demos audit <org> gateway-db

# View the git ledger log
g8e demos audit <org> ledger-log
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

The `swarm` demo includes scenario descriptions in `demos/swarm/README.md`. Swarm scenarios are not integrated into the `g8e demos run` CLI command.

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

The Dockerfile at the repository root is shared across all org directories. Docker Compose builds the g8e binary from source inside each container. No pre-built image is required.

## Invariants

The following must hold in every org environment:

1. The Operator is the only process on net_secure. No agent, no gateway surface, no observability service is co-located on net_secure.
2. No named volume is shared between services. Each service owns its own volume.
3. No PKI material is pre-distributed via filesystem. Identity propagates through enrollment over mTLS.
4. Doctrine is a bind mount, not baked into an image. Org behavior is data, not code.
5. The Dockerfile at the repository root is the only build artifact shared across org directories. Each compose file references `build: context: ../..` to compile the g8e binary from source inside the container. No pre-built binary is bind-mounted.

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

## PKI and Enrollment

There is no shared PKI volume. There is no init container that pre-populates certificates for downstream services.

**Sequence:**

1. Gateway container starts, generates its own CA, initializes PKI under its named volume, begins listening for enrollment on the mTLS port
2. Operator container starts, reads its config (gateway endpoint + device-link token), dials the Gateway over mTLS, completes enrollment, receives its identity certificate
3. From that point forward the Operator holds its own identity material in its own named volume

`depends_on` with `condition: service_healthy` enforces ordering. The Gateway health check (`/healthz`) must pass before the Operator starts.

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

## License

Apache 2.0
