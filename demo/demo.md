# g8e Demo System

The `g8e` demo system provides a modular, sandboxed environment for evaluating AI operations against simulated edge fleets. It uses Docker containers to emulate lightweight edge devices, allowing you to test g8e's discovery, deployment, and remediation capabilities without physical hardware.

## Core Concepts

- **Profiles**: Modular configurations defining fleet size, taxonomy, and failure modes.
- **Node Simulation**: Each "device" is a container running a simulated edge environment.
- **Platform Alignment**: The g8e platform (Dashboard, Engine, Operator) runs host-native, while the demo fleet runs in Docker.

## Quick Start (Default: ACME Corp)

The **ACME Corp Global Fleet** is the standard demo profile, simulating a mixed-enterprise environment with realistic failure modes.

```bash
# Start and authenticate the demo with 100 nodes (default)
./g8e demo deploy -d dlk_your_token

# Start with a specific node count
./g8e demo deploy -n 250 -d dlk_your_token
```

### Fleet Taxonomy (ACME Corp)

Nodes are generated with names and environments reflecting a global enterprise:
- **Retail**: `pos-store-*`, `kiosk-airport-*`, `camera-store-*`
- **Factory**: `sensor-factory-*`, `controller-factory-*`
- **Warehouse**: `scanner-warehouse-*`, `sensor-warehouse-*`
- **Office**: `printer-office-*`, `camera-hq-*`, `badge-office-*`
- **Network/DC**: `gateway-branch-*`, `router-hub-*`, `logger-dc-*`

### Failure Modes

By default, ~5% of nodes are initialized in "broken" states (e.g., bad upstream configs, expired certs, or crash loops). These are designed to be diagnosed and fixed by g8e's AI agents.

## Managing Profiles

Profiles are stored in `demo/profiles/`. You can list and switch between them using the `profile` subcommand.

```bash
# List available profiles
./g8e demo profile list

# Switch to the 'fleet' profile
./g8e demo profile switch fleet
```

### Available Profiles

| Profile | Description |
| :--- | :--- |
| **acme-corp** | **(Default)** Realistic enterprise taxonomy, 100-1000 nodes, mixed failure modes. |
| **fleet** | Minimalist simulator optimized for high-density testing (up to 2000+ nodes). |
| **nginx** | Original 10-node "Broken Web App" demo. |

## Operational Commands

The following commands are available for the active profile:

| Command | Description |
| :--- | :--- |
| `deploy` | Start the fleet and push/start g8e operators (requires `-d <token>`). |
| `down` | Stop and remove fleet containers. |
| `status` | Show health and running state of the fleet. |
| `devices` | List all discovered device hostnames. |
| `broken` | List devices currently in a non-healthy state. |
| `vanish` | Remove all g8e operators and logs from the fleet (zero trace). |
| `shell N=<name>` | Drop into a specific device's shell. |

## Architecture

- `demo/profiles/`: Implementation directories for each profile.
- `demo/Makefile`: The dispatcher that delegates commands to the active profile's own Makefile.
- `demo/profiles/.active`: A state file tracking the currently selected profile.

**Invariants:**
- Demo nodes are always containers labeled with `demo.service`.
- The `deploy` command automatically builds and starts the fleet if not already running.
- Use the `-d` flag with `deploy` to provide a `DEVICE_TOKEN` for operator authentication.
