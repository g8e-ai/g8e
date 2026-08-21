# Finance Trading Controls Demo

This demo simulates a **governed trading floor** where g8e enforces dual-control authorization, transaction limits, and market manipulation detection on every trade execution.

## Overview

The finance demo demonstrates:

- **8 trading control doctrine rules** evaluated on every transaction
- **Network isolation** preventing unauthorized access from net_untrusted to the trading ledger
- **Dual-control authorization** enforcement (no single-approval bypass)
- **Market manipulation detection** (spoofing, layering, wash trades, pump and dump, front running)
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
- **Market Integrity**: Market manipulation (spoofing, layering, wash trade, pump and dump, front running), insider trading
- **Settlement**: Settlement risk violation
- **Audit**: Audit trail bypass detection

## Quick Start

### Prerequisites

- Docker and Docker Compose installed
- g8e binary built at repository root

### Build the g8e binary

```bash
make build
```

This builds the g8e binary and copies it to `demos/bin/g8e`.

### Start the finance demo

```bash
cd demos/finance
docker compose up -d --build
```

All services start, but the operator and any service that depends on it (`target-system`, `agent-runtime`) remain not-ready until their owner-approved platform enrollment requests are approved. Do not use `docker compose up --wait` before approval; it is expected to time out while enrollment is pending.

#### Owner-approved platform activation

After `docker compose up -d --build`, the gateway is healthy but the operator and its dependents are not. Activate them by enrolling the first owner and approving the operator's pending enrollment request:

```bash
# 1. Wait for the gateway to be healthy (the finance demo gateway listens on port 8082).
until curl -fsS http://localhost:8082/api/v1/health >/dev/null 2>&1; do sleep 2; done

# 2. Enroll the first owner. This creates the first user and a usable CLI mTLS identity.
./g8e auth enroll user -e https://localhost:8445

# 3. List pending platform enrollment requests.
./g8e auth pending-platform-enrollments

# 4. Approve the operator's request by exact request ID.
./g8e auth approve-platform-enrollment <operator-request-id> --yes

# 5. Wait for the operator and its dependents to become healthy.
docker compose ps
```

The `g8e demos start finance` CLI path prints these activation instructions automatically, including the demo gateway port and the exact `g8e auth approve-platform-enrollment <request-id>` command to run.

Alternatively, use the g8e CLI from the repository root:

```bash
g8e demos start finance
```

### Check service status

```bash
docker compose ps
```

Or via the g8e CLI:

```bash
g8e demos status finance
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

The `g8e demos run` command automatically starts the demo environment if it is not already running. Use `-v` for verbose step-by-step output, or `--tui` for the tactical governance TUI overlay.

### Scenario 1: Unauthorized Trade Blocked

```bash
g8e demos run finance 1
```

**Proves**: Two-layer defense against unauthorized trading.

The scenario runs a 5-step flow:

1. **Gateway health check**: confirms the g8e gateway is live
2. **Operator enrollment verification**: confirms mTLS certs exist
3. **Demo scenario mTLS transaction**: submits a `finance-unauthorized-trade` scenario via the real gateway; L1 doctrine blocks the unauthorized trade execution payload at confidence >= 0.90
4. **Audit log tail**: verifies doctrine rejection in gateway logs
5. **Network isolation proof**: bad-actor on net_untrusted has no route to net_secure (supplementary proof)

### Run all scenarios

```bash
g8e demos run finance
```

## Architecture Notes

### Gateway Posture: Doctrine

The gateway runs in doctrine mode (the default), meaning:
- L1 Doctrine validation is **enforced** (fail-closed)
- L2 Consensus signatures are audited but not required
- L3 Notary proofs are audited but not required

Layers L4 (Warden) and L5 (Actuator) are always active regardless of posture, providing pre-dispatch verification and signed receipt production on every transaction.

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

Or via the g8e CLI:

```bash
g8e demos stop finance
g8e demos clean finance  # removes containers, volumes, and networks
```

## License

Business Source License 1.1 (BSL 1.1). Converts to Apache 2.0 on 2030-08-18.
