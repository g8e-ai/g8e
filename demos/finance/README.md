# Finance Trading Controls Demo

This demo simulates a **governed trading floor** where g8e enforces dual-control authorization, transaction limits, and market manipulation detection on every trade execution.

## Overview

The finance demo demonstrates:

- **8 trading control doctrine rules** evaluated on every transaction
- **Network isolation** preventing unauthorized access from net_untrusted to the trading ledger
- **Dual-control authorization** enforcement (no single-approval bypass)
- **Market manipulation detection** (spoofing, layering, wash trades, front running)
- **Insider trading detection** via MNPI pattern matching
- **Audit trail integrity** with hash-chained ledger receipts

## Network Topology

- **net_untrusted (10.20.0.0/24)**: Bad actor simulation (unauthorized trade attempts)
- **net_perimeter (10.21.0.0/24)**: Gateway public surface + demo UI
- **net_internal (10.22.0.0/24)**: Agent runtime, LLM backend, operator outbound mTLS
- **net_secure (10.23.0.0/24)**: Operator actuator boundary + target trading system
- **net_mgmt (10.24.0.0/24)**: Observability and audit trail

## Port Mappings

| Service | Port |
|---------|------|
| Gateway HTTP | 8082 |
| Gateway HTTPS | 8445 |
| Console | https://localhost:8445/console/ |
| Demo UI | 3002 |

## Doctrine Rules

The demo includes 8 trading control doctrine rules covering:

- **Trading Limits**: Transaction limit exceeded, position limit violation
- **Authorization**: Dual control bypass, unauthorized trade execution
- **Market Integrity**: Market manipulation (spoofing, layering, wash trade, front running), insider trading
- **Settlement**: Settlement risk violation
- **Audit**: Audit trail bypass detection

## Quick Start

### Prerequisites

- Docker and Docker Compose installed
- g8e binary built at repository root

### Build the g8e binary

```bash
cd /home/bob/g8e
make build
```

### Start the finance demo

```bash
cd demos/finance
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

### Scenario 1: Unauthorized Trade Blocked

```bash
g8e demos run finance 1
```

**Proves**: Two-layer defense against unauthorized trading.

The scenario runs a 5-step flow:

1. **Gateway health check** — confirms the g8e gateway is live
2. **Operator enrollment verification** — confirms mTLS certs exist
3. **Demo scenario mTLS transaction** — submits a `finance-unauthorized-trade` scenario via the real gateway; L1 doctrine blocks the unauthorized trade execution payload at confidence >= 0.90
4. **Audit log tail** — verifies doctrine rejection in gateway logs
5. **Network isolation proof** — bad-actor on net_untrusted has no route to net_secure (supplementary proof)

### Run all scenarios

```bash
g8e demos run finance
```

## Architecture Notes

### Gateway Posture: Consensus

The gateway runs in `--posture consensus` mode, meaning:
- L1 Doctrine validation is **enforced** (fail-closed)
- L2 Consensus signatures are **enforced** (fail-closed, quorum ≥ 2)
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
