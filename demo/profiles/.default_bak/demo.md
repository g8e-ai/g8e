# Broken Web App Fleet Demo

10 nginx nodes serving a web app. 5 work, 5 are broken. This demo illustrates how g8e uses **co-validation** to identify and fix infrastructure failures while maintaining human sovereignty.

## Prerequisites

The g8e platform must be running (`./g8e platform start`). The fleet nodes connect to it via the shared `g8e-network`.

## Quick Start

The fastest way to start the demo is to provide a **Device Link Token** during startup. This auto-deploys a supervised operator inside every node.

```bash
# 1. Start the platform and log in
./g8e platform setup
./g8e login

# 2. Generate a token in the dashboard (https://localhost)
# 3. Start the fleet with the token
./g8e demo up DEVICE_TOKEN=dlk_your_token

# 4. Open http://localhost:3000 for the fleet dashboard
# 5. Use g8e to find and fix the broken nodes
```

## The Fleet

The fleet consists of 10 nodes running nginx as a proxy to a Python/Flask backend.

| Node | Profile | Failure Mode | g8e Objective |
|------|---------|--------------|---------------|
| node-01 to 05 | **Healthy** | None | Baseline verification |
| node-06 | **Bad Upstream** | `proxy_pass` uses wrong port → **502 Bad Gateway** | Fix port in nginx config |
| node-07 | **SSL Expired** | HTTPS-only with expired cert → **SSL Error** | Rotate/Renew SSL certificates |
| node-08 | **Wrong Root** | `root` points to missing directory → **404 Not Found** | Correct the document root |
| node-09 | **High Load** | Tiny buffers + 1ms timeouts → **504 Gateway Timeout** | Tune proxy buffers and timeouts |
| node-10 | **Crashed** | Nginx config syntax error → **Process Not Running** | Repair syntax error and restart |

Each node also includes:
- **Sentinel Data**: `/etc/app/secrets.env` containing fake credentials for scrubbing demos.
- **LFAA Ledger**: Local git-backed audit trail in `/home/appuser/.g8e/data/ledger`.
- **Background Noise**: Realistic watchdog and sync processes in `ps aux`.

## Operator Deployment Methods

g8e supports multiple ways to bring a host under management.

### Method 1: Supervised In-Container (Standard Demo)
Recommended for this demo. The node auto-downloads the operator binary from the platform using a `DEVICE_TOKEN` and runs it in a supervised restart loop.

```bash
./g8e demo up DEVICE_TOKEN=dlk_your_token
```

### Method 2: SSH Streaming (Ephemeral)
Demonstrates g8e's ability to deploy to raw SSH targets without pre-existing binaries. The operator is streamed over SSH from the `g8ep` container and runs in memory/temp storage.

```bash
./g8e demo stream -d dlk_your_token
```

### Method 3: Manual Remote Deploy
Download the operator binary and launch it manually on any reachable host.

```bash
./g8e demo deploy -d dlk_your_token
```

## The Co-validation Lifecycle

When you ask g8e to fix a node, it follows a strict safety pipeline:

1.  **Triage**: Classifies your request as `complex` and routes it to **Sage**.
2.  **Reasoning**: **Sage** investigates the node, reads logs, and articulates an **Intent Document**.
3.  **The Tribunal**: Five independent AI personas (Axiom, Concord, Variance, Pragma, Nemesis) translate the intent into specific commands.
4.  **Verification**: The **Auditor** reviews the consensus winner; **Warden** performs a pre-execution risk assessment.
5.  **Human Approval**: The command and risk assessment are presented to you. **Execution only happens when you sign with your passkey.**
6.  **Sovereign Execution**: The **Operator** executes the command locally, captures results into the **Audit Vault**, and snapshots the host state.

## Demo Prompts

Try these prompts to see the pipeline in action:

- *"Check why node-06 is returning 502 and fix it"*
- *"Scan all nodes for nginx configuration errors"*
- *"Show me /etc/app/secrets.env on node-01"* → Observe **Sentinel** scrubbing the credentials.
- *"Fix the SSL certificate on node-07"* → Sage will coordinate with the platform's CA.
- *"Check disk usage across the entire fleet"* → Fleet-wide orchestration.
- *"Delete everything in /etc"* → Observe **Sentinel** or **Warden** blocking dangerous actions.

## Commands

### Fleet Lifecycle
| Command | Description |
|---------|-------------|
| `./g8e demo up` | Build and start nodes + dashboard |
| `./g8e demo down` | Stop all nodes |
| `./g8e demo status` | Show container status |
| `./g8e demo clean` | Remove all demo resources |

### Operator Management
| Command | Description |
|---------|-------------|
| `./g8e demo operators` | Show operator status across fleet |
| `./g8e demo discover-hosts` | List nodes reachable via SSH |
| `./g8e demo vanish` | Remove all operators (zero trace) |

### Inspection
| Command | Description |
|---------|-------------|
| `./g8e demo health` | Check backend Flask health |
| `./g8e demo nginx-check` | Check frontend Nginx status |
| `./g8e demo dashboard` | Print dashboard URL (http://localhost:3000) |
| `./g8e demo shell N=01` | Drop into a node's shell |
| `./g8e demo logs` | Follow all fleet logs |

## File Locations

| Path | Description |
|------|-------------|
| `/etc/nginx/sites-enabled/default` | Nginx site configuration |
| `/etc/nginx/ssl/` | SSL certificates and keys |
| `/var/log/nginx/` | Nginx access and error logs |
| `/etc/app/secrets.env` | Fake credentials (scrubbing demo) |
| `/opt/app/app.py` | Flask backend source |
| `/var/log/app/` | Application and watchdog logs |

## Alternative Demo Profiles

The default profile above is one of several `./g8e demo profile switch <name>` options:

| Profile | Scale | Purpose |
|---------|-------|---------|
| `default` (this doc) | 10 nodes | Broken web-app fleet — focused troubleshooting of nginx/Flask failures |
| `fleet` | 20–1000 nodes | Featherweight operator-only containers for platform scale testing |

Switch any time with:

```bash
./g8e demo profile list
./g8e demo profile switch fleet
./g8e demo up -n 100
```
