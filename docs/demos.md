---
title: Demos
---

# g8e Demos

Last Updated: 2026-05-18

The `g8e` demo system provides a modular, sandboxed environment for evaluating AI operations against simulated edge fleets. It uses Docker containers to emulate lightweight edge devices, allowing you to test g8e's discovery, deployment, and remediation capabilities without physical hardware.

The demo platform itself runs **host-native** (Operator + optional g8ee), while the simulated fleet runs in Docker.

---

## Quick Start

```bash
# Start and authenticate the default demo (ACME Corp, 100 nodes)
./g8e demo deploy -d dlk_your_token

# Specific node count
./g8e demo deploy -n 250 -d dlk_your_token
```

Generate the `dlk_…` device-link token from the running platform first.

---

## Where the Demos Live

The runnable demo assets are sourced from `/demo` in the repo:

- **`@/home/bob/g8e/demo/demo.md`** - Full demo system reference: profiles, fleet taxonomy, failure modes, operational commands, and troubleshooting.
- **`@/home/bob/g8e/demo/profiles/`** - Profile directories. Each profile defines fleet size, device taxonomy, and failure modes.
  - `acme-corp/` - Default profile. Realistic enterprise taxonomy (retail, factory, warehouse, office, network/DC), 100–1000 nodes, mixed failure modes.
  - `fleet/` - Minimalist simulator optimized for high-density testing (up to 2000+ nodes).
  - `nginx/` - Original 10-node "Broken Web App" demo.
- **`@/home/bob/g8e/demo/Makefile`** - Profile build targets.
- **`@/home/bob/g8e/demo/.active`** - Marker file for the currently selected profile.

---

## Common Commands

| Command | Purpose |
|---|---|
| `./g8e demo deploy -d <token>` | Start and authenticate the active fleet. |
| `./g8e demo down` | Stop all simulation nodes. |
| `./g8e demo status` | Container status and node counts. |
| `./g8e demo clean` | Forcefully remove all demo artifacts. |
| `./g8e demo profile list` | List available profiles. |
| `./g8e demo profile switch <name>` | Switch the active profile. |
| `./g8e demo shell <node>` | Drop into a simulation node's shell. |
| `./g8e demo devices` | List discovered devices. |
| `./g8e demo broken` | List unhealthy devices. |
| `./g8e demo operators` | Status of g8e operator processes inside the fleet. |

For full details on profiles, taxonomy, failure modes, and customization, see `@/home/bob/g8e/demo/demo.md` and the profile READMEs under `@/home/bob/g8e/demo/profiles/`.

See also: [Scripts](scripts.md), [Operator](operator.md).
