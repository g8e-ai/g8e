# Drone Swarm Demo

This demo simulates a battlefield drone swarm with 20 autonomous operators, each running a simulated drone with real-time telemetry and battlefield intelligence data.

## Overview

The swarm demo demonstrates:
- **20 autonomous drone operators** running in Docker containers
- **4 drone types**: Recon (8), Attack (6), Support (4), Relay (2)
- **Battlefield intelligence** with sectors, enemy positions, and no-fly zones
- **Doctrine-based governance** for weapons control, safety, and command integrity
- **Real-time telemetry** including altitude, coordinates, battery, and sensor data
- **Adversary simulation** attempting to intercept drone communications

## Fleet Composition

| Drone Type | Count | Role |
|------------|-------|------|
| Recon | 8 | Surveillance and intelligence gathering |
| Attack | 6 | Offensive operations with weapon systems |
| Support | 4 | Logistics and medical support |
| Relay | 2 | Communications relay for extended range |

## Network Topology

- **net_untrusted (10.30.0.0/24)**: Adversary simulation
- **net_perimeter (10.31.0.0/24)**: Gateway public surface
- **net_internal (10.32.0.0/24)**: Drone operators and command interface
- **net_secure (10.33.0.0/24)**: Privileged operator operations
- **net_mgmt (10.34.0.0/24)**: Observability and monitoring

## Port Mappings

| Service | Port |
|---------|------|
| Gateway HTTP | 8085 |
| Gateway HTTPS | 8448 |
| Console | https://localhost:8448/console/ |
| Command Interface | 5005 |

## Doctrine Rules

The demo includes 15 doctrine rules covering:
- **Weapons Control**: Unauthorized weapon release prevention
- **Safety**: Friendly fire and civilian casualty risk detection
- **Navigation**: Restricted airspace, GPS spoofing, altitude/speed violations
- **Command Control**: Authorization override, autonomous mode restrictions
- **Data Security**: Exfiltration prevention, sensor data tampering
- **Mission Control**: Breach detection, swarm coordination protection

## Quick Start

### Prerequisites

- Docker and Docker Compose installed
- g8e binary built at repository root

### Build the g8e binary

```bash
cd /home/bob/g8e
make build
```

### Start the swarm

```bash
cd demos/swarm
docker compose up -d
```

### Check service status

```bash
docker compose ps
```

### View logs

```bash
# Gateway logs
docker compose logs -f gateway

# Specific drone operator logs
docker compose logs -f operator-1

# All operators
docker compose logs -f operator-1 operator-2 operator-3
```

### View observability

```bash
docker compose logs -f observability
```

### Stop the swarm

```bash
docker compose down
```

### Clean up all runtime state

```bash
docker compose down -v
```

## Running the Drone Simulator

Each operator container has the drone simulator mounted. To run it manually on a specific operator:

```bash
docker compose exec operator-1 python /app/drone_simulator.py DRONE-001 recon
```

## Battlefield Data

The demo includes simulated battlefield intelligence in `target-data/`:

- **battlefield_intel.json**: Mission sectors, enemy/friendly positions, no-fly zones, rules of engagement
- **drone_fleet_manifest.json**: Complete fleet roster with drone types, status, and positions

## Demo Scenarios

The swarm demo includes 3 real scenarios integrated into the `g8e demos run` CLI command. Each scenario submits a real `GovernanceEnvelope` through the gateway via mTLS using `demos scenarios run`.

### Scenario 1: Authorized Recon Mission (Governed Drone Deployment)
```bash
g8e demos run swarm 1
```
Submits a `GovernanceEnvelope` for a drone recon mission through the gateway via `harnessRun("swarm-recon-mission")`. Verifies gateway health, operator enrollment, doctrine loaded, L2 consensus admission, and ledger receipt.

### Scenario 2: Weapons Safety Doctrine Block
```bash
g8e demos run swarm 2
```
Runs `harnessRun("swarm-weapon-release-block")` which attempts an unauthorized weapon release via MCP tool call. Verifies L1 Doctrine blocks it before any operator execution.

### Scenario 3: Navigation Boundary Violation Block
```bash
g8e demos run swarm 3
```
Runs `harnessRun("swarm-restricted-airspace-block")` which attempts to navigate a drone into restricted airspace via MCP tool call. Verifies L1 Doctrine blocks the navigation command.

Use `-v` for verbose step-by-step output:
```bash
g8e demos run swarm 1 -v
```

## Troubleshooting

### Gateway health check failing

```bash
docker compose logs gateway
```

### Operator enrollment failing

```bash
docker compose logs operator-1
```

### Drone simulator not running

The drone simulator is mounted but not executed by default. To run it on specific operators, use the exec command shown above.

### High memory usage

With 20 operators, this demo requires significant resources. If you experience issues, reduce the number of operators in compose.yml.

## Architecture

```
┌─────────────────────────────────────────────────────────────┐
│                     net_untrusted                            │
│  ┌──────────────┐                                            │
│  │   Adversary  │  Attempts to intercept communications     │
│  └──────────────┘                                            │
└─────────────────────────────────────────────────────────────┘
                              │
┌─────────────────────────────────────────────────────────────┐
│                    net_perimeter                             │
│  ┌──────────────┐                                            │
│  │   Gateway    │  HTTP:8085, HTTPS:8448                    │
│  └──────────────┘                                            │
└─────────────────────────────────────────────────────────────┘
                              │
┌─────────────────────────────────────────────────────────────┐
│                    net_internal                              │
│  ┌──────────┐  ┌──────────┐  ┌──────────┐  ┌──────────┐   │
│  │ Operator │  │ Operator │  │ Operator │  │ Command  │   │
│  │   1-20   │  │  Relay   │  │ Support  │  │ Interface│   │
│  └──────────┘  └──────────┘  └──────────┘  └──────────┘   │
└─────────────────────────────────────────────────────────────┘
                              │
┌─────────────────────────────────────────────────────────────┐
│                     net_secure                               │
│  ┌──────────┐  ┌──────────┐  ┌──────────┐                   │
│  │ Operator │  │ Operator │  │ Operator │  Privileged ops  │
│  │  1-20    │  │  Attack  │  │  Recon   │                   │
│  └──────────┘  └──────────┘  └──────────┘                   │
└─────────────────────────────────────────────────────────────┘
                              │
┌─────────────────────────────────────────────────────────────┐
│                      net_mgmt                                 │
│  ┌──────────────┐                                           │
│  │Observability │  Log aggregation and audit trail          │
│  └──────────────┘                                           │
└─────────────────────────────────────────────────────────────┘
```

## License

Apache 2.0
