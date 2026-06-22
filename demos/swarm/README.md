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

### Scenario 1: Normal Operations
All 20 drones operate within doctrine rules, conducting reconnaissance and surveillance missions.

### Scenario 2: Adversary Interception
The adversary service attempts to intercept drone communications. Doctrine rules detect and block unauthorized access attempts.

### Scenario 3: Safety Violation Detection
Simulate a drone attempting to enter a no-fly zone or target friendly forces. Doctrine rules trigger alerts and prevent violations.

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
