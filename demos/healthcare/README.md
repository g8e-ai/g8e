# Healthcare Prior Authorization Demo

This demo simulates a **FHIR R4-compliant prior authorization (PA) workflow** governed by g8e, demonstrating PHI/HIPAA doctrine enforcement, gold-card auto-approval (HB 3134), SLA breach tracking (2026 CCO Medicaid Rule), and bad-actor PHI exfiltration prevention.

## Overview

The healthcare demo demonstrates:

- **FHIR R4 PA submission** through the g8e gateway with real doctrine enforcement
- **11 PHI/HIPAA doctrine rules** evaluated on every request
- **Gold card auto-approval** (HB 3134 §6) for providers with historic approval rate ≥ 90%
- **SLA breach tracking** with day-5 alerts and day-7 breach flags for mandatory DCBS/OHA reporting
- **Two-layer PHI defense**: network isolation + doctrine enforcement against exfiltration
- **Metabase compliance dashboards** pre-loaded with DCBS/OHA filing queries

## Network Topology

- **net_untrusted (10.20.0.0/24)**: Bad actor simulation (PHI exfiltration attempts)
- **net_perimeter (10.21.0.0/24)**: Gateway public surface + compliance dashboard
- **net_internal (10.22.0.0/24)**: PA submission service, agent runtime, LLM backend, operator outbound mTLS
- **net_secure (10.23.0.0/24)**: Operator actuator boundary, exemption rules, PA worker, message broker, reporting DB
- **net_mgmt (10.24.0.0/24)**: Observability and audit trail

## Port Mappings

| Service | Port |
|---------|------|
| Gateway HTTP | 8081 |
| Gateway HTTPS | 8444 |
| Console | https://localhost:8444/console/ |
| RabbitMQ Management | 15673 |
| Reporting DB (Postgres) | 5433 |
| Compliance Dashboard (Metabase) | 3001 |

## Doctrine Rules

The demo includes 11 PHI/HIPAA doctrine rules covering:

- **PHI Detection**: Patient data detection, exfiltration attempts, cross-boundary transfers
- **HIPAA Compliance**: Minimum necessary violation, encryption violation, audit logging bypass
- **PA Authorization**: Approval bypass, gold card exemption bypass, FHIR resource tampering
- **SLA Integrity**: Timestamp manipulation, de-identification failure

## Quick Start

### Prerequisites

- Docker and Docker Compose installed
- g8e binary built at repository root

### Build the g8e binary

```bash
cd /home/bob/g8e
make build
```

### Start the healthcare demo

```bash
cd demos/healthcare
docker compose up -d
```

### Check service status

```bash
docker compose ps
```

### View logs

```bash
# Gateway logs (doctrine enforcement)
docker compose logs -f gateway

# Operator logs (actuator execution)
docker compose logs -f operator

# PA submission service
docker compose logs -f pa-submission-service

# Observability / audit trail
docker compose logs -f observability
```

### Access the compliance dashboard

Open http://localhost:3001 in your browser.

- Login: `admin@g8e.local` / `Metabase1!`
- Pre-loaded queries: DCBS March 1 Filing (Denial Rates by Request Type), OHA March 31 Filing (Median Decision Time)

## Demo Scenarios

### Scenario 1: Authorized Agent Submits a FHIR PA Request

```bash
g8e demos run healthcare 1
```

**Proves**: An authorized agent on net_internal submits a FHIR ClaimResponse through the g8e gateway. Every request passes through the doctrine engine (11 PHI/HIPAA rules) before reaching the PA API backend.

### Scenario 2: Gold Card Auto-Approval (HB 3134 §6)

```bash
g8e demos run healthcare 2
```

**Proves**: Providers whose historic approval rate meets or exceeds the plan threshold (90%) are auto-approved without manual review. PA-2026-0043 (Dr. Priya Nair, 96%) is the proof case.

### Scenario 3: SLA Breach and OHA Reporting (2026 CCO Medicaid Rule)

```bash
g8e demos run healthcare 3
```

**Proves**: The PA worker tracks days-elapsed per request and flags breaches for mandatory DCBS/OHA annual reporting. PA-2026-0044 (Dr. James O'Brien, 10 days) is the proof case.

### Scenario 4: Bad Actor PHI Exfiltration Blocked

```bash
g8e demos run healthcare 4
```

**Proves**: Two-layer defense — Layer 1: network isolation (bad-actor on net_untrusted has no route to net_internal/net_secure). Layer 2: doctrine enforcement (phi_exfil_attempt at confidence 0.95).

### Run all scenarios

```bash
g8e demos run healthcare
```

## Architecture Notes

### Gateway Posture: Consensus

The gateway runs in `--posture consensus` mode with `--mcp-downstream-url http://healthcare-pa-api:8000`, meaning:
- L1 Doctrine validation is **enforced** (fail-closed)
- L2 Consensus signatures are **enforced** (fail-closed, quorum ≥ 2)
- L3 Notary proofs are audited but not required

### Data Sovereignty

All audit data is committed locally:
- **Git-backed ledger**: Immutable execution history on the operator container
- **SQLite audit vault**: Queryable audit trail on both gateway and operator
- **Postgres reporting DB**: Compliance metrics for DCBS/OHA annual reporting
- No data leaves the demo environment unless explicitly transmitted

## Stop and Clean Up

```bash
docker compose down
docker compose down -v  # also removes volumes
```

## License

Apache 2.0
