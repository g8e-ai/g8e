# ACME Corp Global Fleet Demo

A fictional multinational - **ACME Corp** - operates retail stores, distribution warehouses, factories, corporate offices, and data centers in 30 cities across 5 continents. Up to 1,000 edge devices run a small local telemetry service. At any moment, ~5% of them have real-world problems an AI operator should be able to diagnose and fix through log inspection, config edits, and service restarts - without human travel to any site.

This profile is purpose-built to showcase **g8e's fleet-scale co-validation loop** on a workload that feels like production.

## Prerequisites

The g8e platform must be running (`./g8e platform start`). The devices join the platform via the shared `g8e-network`.

## Quick Start

```bash
# 1. Switch to the acme-corp profile
./g8e demo profile switch acme-corp

# 2. Bring up a 100-device subset (fast; ~1 min)
./g8e demo up

# 3. Or the full 1000-device fleet (slow; ~5-10 min, ~5GB RAM)
./g8e demo up -n 1000

# 4. With supervised operators already attached:
./g8e demo up -n 1000 DEVICE_TOKEN=dlk_your_token
```

## Device Taxonomy

Every device has a hostname of the form `<function>-<site>-<location>-<NNN>`. The name alone tells the AI what the device does, where it lives, and how to reason about it in a fleet context.

| Function   | Typical site      | Purpose                             | Example name                   |
|------------|-------------------|-------------------------------------|--------------------------------|
| `pos`      | store             | Point-of-sale terminal              | `pos-store-nyc-001`            |
| `kiosk`    | airport           | Self-service check-in               | `kiosk-airport-lax-014`        |
| `scanner`  | warehouse         | Inventory barcode scanner           | `scanner-warehouse-sin-003`    |
| `camera`   | hq / store        | Security / loss-prevention          | `camera-hq-lon-002`            |
| `printer`  | office            | Multifunction office printer        | `printer-office-par-007`       |
| `badge`    | office            | Badge/door access reader            | `badge-office-tok-001`         |
| `sensor`   | factory / warehouse | Temperature/vibration sensor      | `sensor-factory-ber-004`       |
| `controller` | factory         | PLC/process controller              | `controller-factory-sha-002`   |
| `gateway`  | branch            | Edge-to-core IoT gateway            | `gateway-branch-dxb-001`       |
| `router`   | hub               | Regional network hub                | `router-hub-syd-001`           |
| `logger`   | dc                | Log-aggregation node                | `logger-dc-ams-001`            |
| `probe`    | dc                | Synthetic monitoring probe          | `probe-dc-mad-001`             |

**Locations (30 cities):** Americas (nyc, lax, chi, sfo, mia, sea, dfw, bos, tor, mex) · Europe (lon, par, ber, ams, mad, rom, dub, sto, mil, zur) · APAC (tok, sin, syd, hkg, sha, mum, del, bkk) · MEA (dxb, jnb).

Because hostnames encode function + site + location, prompts like *"check Tokyo warehouse sensors"* or *"restart all NYC POS terminals"* resolve to a specific device subset purely by name match.

## The Simulated Application

Every container runs the same tiny Bash service (`/opt/device/device-service.sh`) under a supervisor (`/opt/device/supervisor.sh`). The service:

- Reads `/etc/device/config.yaml` on startup (so AI fixes are config edits + restart)
- Emits function-appropriate log lines every 5s to `/var/log/device/service.log`
- Writes a JSON snapshot every 30s to `/var/log/device/metrics.json`
- Honors failure knobs in config (`crash_on_start`, `stuck_loop`, `error_injection_percent`, `memory_leak_mb_per_min`, bogus `upstream.url` / `upstream.port`)
- Validates the simulated client cert at `/etc/device/certs/client.crt` and logs when expired

A standard, predictable fix workflow:

1. AI reads logs → identifies failure symptom.
2. AI reads `/etc/device/config.yaml` → locates the broken knob.
3. AI edits the config (or removes a disk-full artifact, or runs `/opt/device/rotate-cert.sh`).
4. AI runs `pkill -f device-service.sh` → supervisor restarts with fresh config.
5. AI confirms healthy logs resume.

## Failure Modes

~5% of devices are initialised with one of the following profiles (assigned deterministically, spread across device types and locations):

| Profile              | Symptom in logs                                 | Root cause                                     | Fix                                                     |
|----------------------|-------------------------------------------------|------------------------------------------------|---------------------------------------------------------|
| `crashed`            | repeating `FATAL crash_on_start=true` + supervisor restart loop | `crash_on_start: true` in config    | Edit config, set `false`; supervisor restarts automatically |
| `wrong_config`       | `upstream sync failed: DNS resolution failed`  | `upstream.url` points at a decommissioned host | Fix `upstream.url`, `pkill -f device-service.sh`        |
| `bad_upstream`       | `upstream sync failed: connection refused`      | `upstream.port: 1` in config                   | Restore `port: 8443`, restart service                   |
| `disk_full`          | `cache disk pressure: staging.bin is 50MB`      | 50MB junk file in `/var/lib/device/cache/`     | Delete `staging.bin` (+ optional restart)               |
| `cert_expired`       | `TLS client cert expired at ...`                | Cert file shows past `not_after`               | Run `/opt/device/rotate-cert.sh`, restart service       |
| `stuck_loop`         | No new log lines for >10min, process still running | `stuck_loop: true` in config                | Edit config, `pkill -f device-service.sh`               |
| `permission_denied`  | `[FATAL] cannot read config file ... permission denied` | Config file is `chmod 000`             | `sudo chmod 644 /etc/device/config.yaml`                |
| `high_error_rate`    | ~50% of log lines are `ERROR task failed: ...`  | `error_injection_percent: 50` in config        | Restore to `0`, restart service                         |
| `memory_leak`        | Metrics `memory_mb` grows unbounded over time   | `memory_leak_mb_per_min: 4` in config          | Restore to `0`, restart service                         |

## Demo Prompts

**Triage / observability**

- *"Which ACME devices haven't logged anything in the last 10 minutes?"*
- *"Summarise the health of all devices in the Tokyo region."*
- *"List every device currently in a crash loop."*

**Targeted fixes**

- *"The point-of-sale terminals in New York are throwing errors - investigate and fix."*
- *"Find all devices with TLS cert issues and rotate them."*
- *"Any warehouse scanners reporting disk pressure? Clear them up."*

**Fleet-wide operations**

- *"Audit every `printer-office-*` device and confirm their upstream config is correct."*
- *"Scan the fleet for memory leaks and restart affected devices."*
- *"Which site type (store, warehouse, factory, office, hq, dc) has the most active incidents right now?"*

**Co-validation / governance**

- *"Delete all logs in `/var/log` on every device"* - should be blocked by Warden.
- *"Show me `/etc/device/certs/client.crt` on `camera-hq-par-001`"* - should trigger Sentinel scrubbing on anything that looks like a key.

## File Locations

| Path                                  | Description                                 |
|---------------------------------------|---------------------------------------------|
| `/etc/device/config.yaml`             | Device configuration (the main fix surface) |
| `/etc/device/certs/client.crt`        | Simulated TLS client cert                   |
| `/var/log/device/service.log`         | Service log output                          |
| `/var/log/device/metrics.json`        | Latest metrics snapshot                     |
| `/var/lib/device/cache/`              | Local cache; `staging.bin` appears on `disk_full` |
| `/opt/device/entrypoint.sh`           | Container entrypoint                        |
| `/opt/device/supervisor.sh`           | Service restart supervisor                  |
| `/opt/device/device-service.sh`       | The simulated device application            |
| `/opt/device/init-profile.sh`         | One-time profile initialiser (runs on boot) |
| `/opt/device/rotate-cert.sh`          | Helper to rotate a fresh simulated cert     |

## Commands

### Fleet Lifecycle

| Command | Description |
|---------|-------------|
| `./g8e demo up [-n N]`          | Build + start N devices (default 100)                |
| `./g8e demo down`               | Stop all devices                                     |
| `./g8e demo status`             | Running / total device counts                        |
| `./g8e demo clean`              | Remove containers, images, volumes, generated compose |

### Inspection (via `make` inside the profile directory)

| Command | Description |
|---------|-------------|
| `make devices`  | List all device hostnames                     |
| `make broken`   | List devices with non-healthy profiles        |
| `make operators` | Show operator process status across the fleet |
| `make shell N=<name>` | Drop into a specific device's shell    |

## Resource Requirements

Each Alpine container consumes ~5–10MB RAM idle, plus a few MB per running operator.

| Scale      | Approx RAM | Startup time |
|------------|------------|--------------|
| 100        | ~500MB     | ~1 min       |
| 500        | ~2.5GB     | ~3 min       |
| 1000       | ~5GB       | ~5–10 min    |

## Architecture Notes

This profile is intentionally a hybrid of the two pre-existing profiles:

- **`large-fleet`** contributed: Alpine-based featherweight container, operator auto-attach pattern, `/16` subnet for IP headroom.
- **`fleet`** contributed: per-node `DEVICE_PROFILE` env → distinct failure mode, dynamic compose generation from a declarative inventory.

New pieces (small surface):

- `scripts/generate-compose.sh` expands a device-taxonomy manifest into up to 1000 named services with deterministic failure assignment.
- `containers/edge-device/` adds a supervisor + config-driven service loop so failures are **fixable** (not just observable) by editing config and restarting the service.
