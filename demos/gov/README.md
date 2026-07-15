# Government CUI Protection Demo

This demo simulates a **governed defense contractor environment** where g8e enforces CUI (Controlled Unclassified Information) protection and CMMC (Cybersecurity Maturity Model Certification) compliance on all data access and exfiltration attempts.

## Overview

The gov demo demonstrates:

- **8 CUI/CMMC doctrine rules** evaluated on every transaction
- **Two-layer defense**: network isolation (Layer 1) + doctrine enforcement (Layer 2)
- **Demo scenario mTLS transactions** submitting real GovernanceEnvelopes through the gateway
- **Network isolation** preventing unauthorized access from net_untrusted to net_secure
- **Audit trail integrity** with hash-chained ledger receipts

## Network Topology

- **net_untrusted (10.20.0.0/24)**: Bad actor simulation (CUI exfiltration attempts)
- **net_perimeter (10.21.0.0/24)**: Gateway public surface + demo UI
- **net_internal (10.22.0.0/24)**: Agent runtime, operator outbound mTLS
- **net_secure (10.23.0.0/24)**: Operator actuator boundary + target classified document store
- **net_mgmt (10.24.0.0/24)**: Observability and audit trail

## Port Mappings

| Service | Port |
|---------|------|
| Gateway HTTP | 8080 |
| Gateway HTTPS | 8443 |
| Console | https://localhost:8443/console/ |
| Demo UI | 3000 |

## Doctrine Rules

The demo includes 8 CUI/CMMC doctrine rules covering:

- **Classification Detection**: CUI marking detection, unauthorized classified document access
- **Access Control**: CMMC Level 2 access control requirements
- **Exfiltration Prevention**: CUI exfiltration attempt detection at confidence >= 0.95, unauthorized cross-domain CUI transfer
- **Encryption**: CMMC encryption requirement violation detection
- **Data Storage**: CUI storage location violation detection
- **Audit**: CMMC audit logging bypass detection

## Quick Start

### Prerequisites

- Docker and Docker Compose installed
- g8e binary built and copied to `demos/bin/`

### Build the g8e binary

```bash
make build
```

This builds the g8e binary and copies it to `demos/bin/g8e`.

### Start the gov demo

Using the g8e CLI (recommended):

```bash
g8e demos start gov
```

Or using Docker Compose directly:

```bash
cd demos/gov
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

# Observability / audit trail
docker compose logs -f observability
```

## Demo Scenarios

### Scenario 1: CUI Exfiltration Attempt Blocked

```bash
g8e demos run gov 1
```

**Proves**: Two-layer defense against CUI exfiltration.

The scenario runs a 5-step flow:

1. **Gateway health check**: confirms the g8e gateway is live
2. **Operator enrollment verification**: confirms mTLS certs exist
3. **Demo scenario mTLS transaction**: submits a `gov-cui-exfil-block` scenario via the real gateway; L1 doctrine blocks the CUI exfiltration payload at confidence >= 0.95
4. **Audit log tail**: verifies doctrine rejection in gateway logs
5. **Network isolation proof**: bad-actor on net_untrusted has no route to net_secure (supplementary proof)

### Run all scenarios

```bash
g8e demos run gov
```

## Architecture Notes

### Gateway Posture: Doctrine

The gateway runs in doctrine mode (the default), meaning:
- L1 Doctrine validation is **enforced** (fail-closed)
- L2 Consensus signatures are audited but not required
- L3 Notary proofs are audited but not required

### Data Sovereignty

All audit data is committed locally:
- **Git-backed ledger**: Immutable execution history on the operator container
- **SQLite audit vault**: Queryable audit trail on both gateway and operator
- No data leaves the demo environment unless explicitly transmitted

## Stop and Clean Up

```bash
docker compose down
docker compose down -v  # also removes volumes
```

## License

Apache 2.0
