# g8e Demo Environments

This directory contains Docker Compose demo environments for org-specific g8e deployments. Each org environment is hermetically sealed with no shared state, volumes, or cross-org dependencies.

## Repository Layout

```
demos/
├── ../g8e                      # single static binary at repository root, bind-mounted into every container
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
│   └── target-data/            # Simulated EHR/PA records
├── finance/                    # Finance/trading demo
│   ├── compose.yml
│   ├── config/
│   ├── doctrine/               # Trading controls and dual-control triggers
│   └── target-data/            # Simulated ledger/positions
└── secure-data/                # Governed data migration / two-operator pipeline demo
    ├── compose.yml
    ├── config/
    ├── doctrine/               # Migration-screening L1 rules (bypass, exfil, cross-tenant)
    └── target-data/            # Simulated SharePoint migration manifest
```

See the [Secure Data Transfer guide](../docs/guides/secure_data_transfer.md) for the
full transport-plane vs. governed-data-plane model this demo illustrates.

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

## Org Differentiation

Each org demonstrates different compliance requirements and use cases:

| Dimension | Gov | Healthcare | Finance |
|---|---|---|---|
| Doctrine content | CUI, classification markings, CMMC rules | PHI scrub patterns, PA workflow gates | Tx limits, dual-control triggers |
| Target data content | Simulated classified document store | Simulated EHR / PA records | Simulated ledger / positions |
| L2 consensus seat count | 3-of-5 | 2-of-3 | 3-of-3 |
| Agent principal type | DoD contractor agent | Clinical AI agent | Algorithmic trading agent |
| Target system mock | Classified doc API | EHR / PA processor | Trade execution API |
| Demo narrative | CUI exfil attempt blocked | PHI scrub + PA approval flow | Unauthorized trade blocked |

## Quick Start

### Prerequisites

- Docker and Docker Compose installed
- g8e binary built at repository root

### Build the g8e binary

From the repository root:

```bash
make build
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

**Finance Demo Scenarios:**
- `g8e demos run finance 1` - Unauthorized Trade Blocked

**Governed Data Migration Demo Scenarios:**
- `g8e demos run secure-data 1` - Governed Migration with Chain-of-Custody Receipts
- `g8e demos run secure-data 2` - Connector Bypass Attempt Blocked
- `g8e demos run secure-data 3` - Cross-Tenant Leak Doctrine Triggered

Note: The demo environment must be started before running scenarios. Use `g8e demos start <org>` first.

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

No images to rebuild. No base files to modify. No other org affected.

## Invariants

The following must hold in every org environment:

1. The Operator is the only process on net_secure. No agent, no gateway surface, no observability service is co-located on net_secure.
2. No named volume is shared between services. Each service owns its own volume.
3. No PKI material is pre-distributed via filesystem. Identity propagates through enrollment over mTLS.
4. Doctrine is a bind mount, not baked into an image. Org behavior is data, not code.
5. The binary is the only artifact shared across org directories. `../g8e` (at repository root) is a bind mount path, not a copied file per org.

## Port Mappings

Each org uses different host ports to allow simultaneous deployment:

| Org | HTTP Port | HTTPS Port | Demo UI Port |
|---|---|---|---|
| gov | 8080 | 8443 | 3000 |
| healthcare | 8081 | 8444 | 3001 |
| finance | 8082 | 8445 | 3002 |
| secure-data (src) | 8083 | 8446 | 3003 |
| secure-data (dst) | 8084 | 8447 | -    |

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
