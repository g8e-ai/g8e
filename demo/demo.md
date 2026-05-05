# g8e Demo System

The `g8e` demo system is modular and supports multiple "profiles". Each profile defines a specific fleet configuration, failure modes, and deployment scenarios.

## Active Profile: ACME Corp Global Fleet

The current default is the **ACME Corp** profile, which provides a robust, region-specific, multi-node environment.

### Quick Start

```bash
# Start the demo with 100 nodes (default)
./g8e demo up

# Start with a specific node count and auto-attach operators
./g8e demo up NODE_COUNT=500 DEVICE_TOKEN=dlk_your_token
```

## Managing Profiles

You can switch between different demo scenarios using the `profile` subcommand.

### List Profiles
```bash
./g8e demo profile list
```

### Switch Profile
```bash
./g8e demo profile switch nginx
```

## Available Profiles

| Profile | Description |
|---------|-------------|
| `acme-corp` | **(Default)** 20-1000 nodes, 4 regions, realistic enterprise taxonomy, fixable failure modes. |
| `nginx` | Original 10-node "Broken Web App" demo. |
| `fleet` | Minimal Alpine-based simulator for high-density testing (up to 1000+ nodes). |

## Architecture

- `demo/profiles/`: Contains the actual implementation of each profile.
- `demo/Makefile`: A dispatcher that delegates commands to the active profile.
- `demo/profiles/.active`: Tracks which profile is currently selected.
