# Broken Web App Fleet Demo

10 nginx nodes serving a web app. 5 work, 5 are broken in different ways. Deploy g8e operators and fix the fleet with AI.

## Prerequisites

The g8e platform must be running (`./g8e platform start` from the project root). The fleet demo nodes connect to it via `g8e.local`.

## Quick Start

```bash
# Start the fleet
./g8e demo up

# Deploy operators (pick one method)
./g8e demo deploy DEVICE_TOKEN=dlk_your_token      # Method 1: API download (device link)
./g8e demo stream DEVICE_TOKEN=dlk_your_token      # Method 2: SSH streaming (recommended)

# Open http://localhost:3000 for the fleet status dashboard
# Use g8e to find and fix broken nodes

./g8e demo vanish                                  # Remove all operators (zero trace)
```

## The Fleet

| Node | Profile | Problem |
|------|---------|---------|
| node-01 to 05 | Healthy | nginx → Flask backend, everything works |
| node-06 | **Bad Upstream** | `proxy_pass` points to wrong port → 502 Bad Gateway |
| node-07 | **SSL Expired** | HTTPS-only with expired self-signed cert |
| node-08 | **Wrong Root** | Document root points to nonexistent directory → 404 |
| node-09 | **High Load** | Tiny proxy buffers + short timeouts → 504 Gateway Timeout |
| node-10 | **Crashed** | nginx config syntax error → nginx won't start at all |

Each node also has:
- `/etc/app/secrets.env` with fake credentials (for Sentinel data scrubbing demo)
- `/etc/app/config.json` with app configuration
- Flask backend on port 5000 (gunicorn)
- SSH server on port 22 (for operator streaming)
- Background watchdog and data-sync processes

## Operator Deployment Methods

### Method 1: API Download (via Device Token)

Each node downloads the operator binary directly from the platform using a device link token. This is the standard deployment path for remote machines that can reach the platform over HTTPS.

The operator is launched with the device link token (`-D`) and an explicit endpoint (`-e g8e.local`).

```bash
./g8e demo deploy DEVICE_TOKEN=dlk_your_token
```

### Method 2: SSH Streaming

The operator binary is streamed over SSH to all nodes concurrently from g8ep. No binary needs to exist on the target machines beforehand. This demonstrates the ephemeral agent deployment capability.

```bash
./g8e demo stream DEVICE_TOKEN=dlk_your_token
```

This command automatically:
1. Extracts the SSH key from the fleet image
2. Configures SSH credentials in g8ep
3. Discovers running demo nodes via Docker labels
4. Streams the operator binary to all discovered nodes

The operator binary at `/home/g8e/g8e.operator` must exist (run `./g8e operator build` first if needed). The SSH key is baked into the fleet demo image at build time. All nodes accept the same key for the `appuser` account.

### Method 3: Operator Deploy UI

Use the g8e dashboard Operator Deploy page to deploy individual operators. The fleet nodes are accessible by hostname on the shared `g8e-network` Docker network (e.g. `web-node-01`).

## Demo Prompts

Things to ask g8e once operators are deployed:

- *"Check if nginx is running and show me the error log"*
- *"Why is this node returning 502?"* → discovers wrong proxy_pass port
- *"Fix the nginx config to proxy to port 5000"* → requires approval
- *"Show me the contents of /etc/app/secrets.env"* → Sentinel scrubs credentials
- *"cat /etc/app/secrets.env"* → Sentinel scrubs AWS keys, DB passwords, API tokens
- *"Delete all nginx logs"* → Sentinel blocks dangerous command
- *"Check disk usage across all operators"* → fleet-wide execution

## Commands

### Fleet Lifecycle
| Command | Description |
|---------|-------------|
| `./g8e demo up` | Build and start all 10 nodes + dashboard |
| `./g8e demo down` | Stop all nodes |
| `./g8e demo status` | Show container status |
| `./g8e demo clean` | Remove everything |

### Operator Deployment
| Command | Description |
|---------|-------------|
| `./g8e demo deploy DEVICE_TOKEN=dlk_xxx` | Deploy operators via API download |
| `./g8e demo stream DEVICE_TOKEN=dlk_xxx` | Deploy operators via SSH streaming (auto-configures SSH) |
| `./g8e demo discover-hosts` | List discovered demo fleet hosts |
| `./g8e demo operators` | Show operator status |
| `./g8e demo vanish` | Remove all operators (zero trace) |

### Inspection
| Command | Description |
|---------|-------------|
| `./g8e demo health` | Check Flask backend health |
| `./g8e demo nginx-check` | Check nginx status + HTTP codes |
| `./g8e demo dashboard` | Print fleet dashboard URL (http://localhost:3000) |
| `./g8e demo shell N=01` | Shell into a specific node |
| `./g8e demo logs` | Follow all container logs |

## File Locations (per node)

| Path | Description |
|------|-------------|
| `/etc/nginx/sites-enabled/default` | nginx site config (the thing that's broken) |
| `/etc/nginx/ssl/` | SSL cert and key |
| `/var/log/nginx/access.log` | nginx access log |
| `/var/log/nginx/error.log` | nginx error log |
| `/etc/app/secrets.env` | Fake credentials (scrubbing demo) |
| `/etc/app/config.json` | App configuration |
| `/var/log/app/` | App logs (gunicorn, watchdog, data-sync) |
| `/var/www/html/index.html` | Static fallback page |
