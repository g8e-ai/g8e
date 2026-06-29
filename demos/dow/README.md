# Department of War (DoW) Tactical Edge Demo

This demo simulates a **SWaP-constrained, air-gapped tactical edge payload** on a Group 2/3 UAS, demonstrating g8e's agentic cross-cueing, BFT consensus, and fail-closed security entirely independent of a ground station.

## Overview

The DoW demo directly addresses **DoW Challenge Areas 5** (Multi-Modal WAPS Payload) and **8** (Alternative PNT), with additional coverage for **Challenge Area 6** (Open-Architecture GOTS Group 3 UAS).

### What It Proves

- **Autonomous SIGINT-to-EO/IR cross-cueing** with cryptographic proof of consensus (Challenge 5)
- **BFT spoofing defense** against near-peer EW/GNSS attacks (Challenge 8)
- **Disconnected operations** with local ledger and audit vault — no cloud, no OEM keys (Challenge 6)
- **Low-SWaP governance**: g8e runs in <128MB RAM and <0.5 CPU per container

## Container Architecture

```
┌──────────────────────────────────────────────────────────────────┐
│                     net_untrusted (10.40.0.0/24)                  │
│  ┌──────────────────┐                                            │
│  │   EW Adversary   │  Spoofed GNSS + datalink jam simulation    │
│  └──────────────────┘                                            │
└──────────────────────────────────────────────────────────────────┘
                              │
┌──────────────────────────────────────────────────────────────────┐
│                    net_perimeter (10.41.0.0/24)                   │
│  ┌──────────┐  ┌──────────────────┐                              │
│  │ Gateway  │  │  Ground Station  │  Simulated C2 datalink       │
│  │ (consens)│  │  (toggleable)    │                              │
│  └──────────┘  └──────────────────┘                              │
└──────────────────────────────────────────────────────────────────┘
                              │
┌──────────────────────────────────────────────────────────────────┐
│                    net_internal (10.42.0.0/24)                    │
│  ┌──────────────┐  ┌──────────────┐  ┌──────────────────┐       │
│  │ agent-sigint │  │ agent-eoir   │  │ agent-pnt-fusion │       │
│  │ (SIGINT)     │  │ (EO/IR cam)  │  │ (PNT BFT)        │       │
│  └──────────────┘  └──────────────┘  └──────────────────┘       │
│  ┌──────────────────────────────────────┐                       │
│  │ Operator (outbound mTLS to Gateway)  │                       │
│  └──────────────────────────────────────┘                       │
└──────────────────────────────────────────────────────────────────┘
                              │
┌──────────────────────────────────────────────────────────────────┐
│                     net_secure (10.43.0.0/24)                     │
│  ┌──────────────────────────────────────┐                       │
│  │ Operator (actuator boundary)         │                       │
│  │ Gimbal controller + flight ctrl      │                       │
│  └──────────────────────────────────────┘                       │
└──────────────────────────────────────────────────────────────────┘
                              │
┌──────────────────────────────────────────────────────────────────┐
│                      net_mgmt (10.44.0.0/24)                      │
│  ┌──────────────────────────────────────┐                       │
│  │ Observability (audit log tail)       │                       │
│  └──────────────────────────────────────┘                       │
└──────────────────────────────────────────────────────────────────┘
```

## Network Topology

- **net_untrusted (10.40.0.0/24)**: EW adversary simulation (spoofed GNSS, datalink jam)
- **net_perimeter (10.41.0.0/24)**: Gateway public surface + simulated ground station
- **net_internal (10.42.0.0/24)**: Sensor agents (SIGINT, EO/IR, PNT Fusion) + Operator outbound mTLS
- **net_secure (10.43.0.0/24)**: Operator actuator boundary (gimbal, flight controller)
- **net_mgmt (10.44.0.0/24)**: Observability and audit trail

## SWaP Constraints

Docker resource limits prove g8e's governance layer fits within tactical payload budgets:

| Service | CPU Limit | Memory Limit | Purpose |
|---------|-----------|-------------|---------|
| gateway | 0.5 | 128M | BFT consensus coordinator |
| operator | 0.5 | 128M | Actuator execution boundary |
| agent-sigint | 0.25 | 64M | SIGINT sensor agent |
| agent-eoir | 0.25 | 64M | EO/IR camera agent |
| agent-pnt-fusion | 0.25 | 64M | PNT fusion agent |

**Governance overhead**: ~20MB binary, ~2W power equivalent. Prove it with `docker stats`.

## Port Mappings

| Service | Port |
|---------|------|
| Gateway HTTP | 8086 |
| Gateway HTTPS | 8449 |
| Console | https://localhost:8449/console/ |

## Doctrine Rules

The demo includes 15 doctrine rules covering:

- **Weapons Control**: Unauthorized weapon release prevention
- **Electronic Warfare**: GPS/GNSS spoofing, communication jamming, unauthorized frequency hop
- **Navigation**: PNT sensor divergence (BFT trigger), restricted airspace, geofence breach
- **Command Control**: Unauthorized cross-cue, autonomous mode override, command override
- **Data Security**: Sensor data tampering, datalink exfiltration
- **Safety**: Battery critical override
- **Resource Control**: SWaP constraint violation
- **Mission Control**: Mission parameters breach

## Quick Start

### Prerequisites

- Docker and Docker Compose installed
- g8e binary built at repository root

### Build the g8e binary

```bash
cd /home/bob/g8e
make build
```

### Start the DoW demo

```bash
cd demos/dow
docker compose up -d
```

### Check service status

```bash
docker compose ps
```

### View logs

```bash
# Gateway logs (BFT consensus)
docker compose logs -f gateway

# Operator logs (actuator execution)
docker compose logs -f operator

# Sensor agent logs
docker compose logs -f agent-sigint
docker compose logs -f agent-eoir
docker compose logs -f agent-pnt-fusion
```

### Verify SWaP constraints

```bash
docker stats dow-gateway dow-operator dow-agent-sigint dow-agent-eoir dow-agent-pnt-fusion
```

### Run sensor agents manually

```bash
# agent-sigint is a real g8e binary — run the cross-cue harness scenario
docker compose run --rm agent-sigint agent-harness run \
  --mtls-url https://g8e.local:8443 \
  --public-url http://g8e.local:8080 \
  --cert /root/.g8e/pki/operator.crt \
  --key /root/.g8e/pki/operator.key \
  --ca /root/.g8e/pki/trust/g8eg-ca-bundle.pem \
  --ensemble 3 --l3-mode mock dow-cross-cue

# agent-eoir and agent-pnt-fusion still use dow_simulator.py for display
docker compose exec agent-eoir python /app/dow_simulator.py EOIR-01 eoir
docker compose exec agent-pnt-fusion python /app/dow_simulator.py PNT-FUSION-01 pnt_fusion
```

## Demo Scenarios

### Scenario 1: Autonomous SIGINT-to-EO/IR Cross-Cueing (Challenge 5)

```bash
g8e demos run dow 1
```

**Proves**: The `agent-sigint` container (a real g8e binary) shares the operator's enrolled mTLS credentials via a read-only volume mount, constructs a `GovernanceEnvelope` wrapping a `run_shell_command` tool call to `slew` the camera, and submits it to the `g8e-gateway`. The Gateway enforces L1 doctrine and L2 BFT consensus (quorum 2/3). The `g8e-operator` verifies the proofs and executes the `slew` script via the L5 Actuator, which sends an HTTP POST to the mock gimbal controller on `net_secure`. The gimbal records the slew — **zero** ground station intervention.

**Key components**:
- **`agent-sigint`**: Real g8e binary running `agent-harness run dow-cross-cue` with the operator's mTLS credentials (shared via `operator_state` volume mount at `/root/.g8e:ro`).
- **`gimbal`**: Mock external (Python HTTP server on `net_secure`) that records camera slew commands.
- **`slew.sh`**: Demo artifact mounted at `/usr/local/bin/slew` in the operator container; wraps the HTTP call to the gimbal, working around `DangerousPatterns` blocking `curl`/`wget` in `run_shell_command`.

### Scenario 2: BFT Spoofing Defense (Challenge 8)

```bash
g8e demos run dow 2
```

**Proves**: A spoofed GNSS coordinate is injected into the PNT fusion engine. The BFT consensus engine detects divergence between the spoofed GNSS source and Visual Odometry/MAGNAV sources. The poisoned model is outvoted. The `GovernanceEnvelope` fails L2 verification, and the `g8e-operator` fails closed — the drone is not hijacked.

### Scenario 3: Disconnected Operations (Challenge 6)

```bash
g8e demos run dow 3
```

**Proves**: The tactical datalink is severed (`docker network disconnect`), simulating a comms-denied environment. The `g8e-gateway` and `g8e-operator` continue processing cross-cueing events locally. Raw data and execution histories are committed to g8e's Git-backed ledger and SQLite local audit vault on the operator container — with no cloud connectivity and no OEM permission keys.

### Run all scenarios

```bash
g8e demos run dow
```

## Tactical Data Files

- **tactical_environment.json**: RF environment, PNT sources (including spoofed), EO/IR payload config, no-fly zones, rules of engagement
- **payload_manifest.json**: Sensor definitions, actuator boundaries, SWaP constraints, governance config, datalink parameters

## Architecture Notes

### Gateway Posture: Consensus

The gateway runs in `--posture consensus` mode, meaning:
- L1 Doctrine validation is **enforced** (fail-closed)
- L2 Consensus signatures are **enforced** (fail-closed, quorum ≥ 2)
- L3 Notary proofs are audited but not required

This matches the tactical edge use case: machine-to-machine autonomy with BFT consensus, no human-in-the-loop required for non-lethal actions.

### Operator: Outbound-Only mTLS

The operator connects to the gateway via outbound-only mTLS tunnel over `net_internal`. It accepts no inbound connections from `net_internal`. The actuator boundary (gimbal controller, flight controller) lives on `net_secure`.

### Data Sovereignty

All audit data is committed locally:
- **Git-backed ledger**: Immutable execution history on the operator container
- **SQLite audit vault**: Queryable audit trail on both gateway and operator
- No data leaves the payload unless explicitly transmitted over the tactical datalink

## Stop and Clean Up

```bash
docker compose down
docker compose down -v  # also removes volumes
```

## License

Apache 2.0
