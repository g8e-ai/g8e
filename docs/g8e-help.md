---
title: g8e Help
---

g8e CLI

AUTHENTICATION
  Authenticate once to save session locally (like AWS/gcloud):
    ./g8e login --api-key <key>
    ./g8e login --device-token <token>

  Logout to clear saved session:
    ./g8e logout

  Alternatively, provide credentials per-command:
    ./g8e <command> --api-key <key>
    ./g8e <command> --device-token <token>

  Platform commands (setup, start, stop, status, rebuild, reset, wipe, clean, logs, settings, update)
  do not require authentication as they manage Docker services directly.

QUICK START
  ./g8e platform setup
  ./g8e platform rebuild
  ./g8e login --api-key <key>
  ./g8e test g8ee tests/unit
  ./g8e operator deploy user@host

COMMON COMMANDS

Platform:
  ./g8e platform setup
  ./g8e platform start
  ./g8e platform stop
  ./g8e platform status
  ./g8e platform rebuild
  ./g8e platform logs [service]

Authentication:
  ./g8e login --api-key <key>
  ./g8e logout

Operator:
  ./g8e operator build
  ./g8e operator deploy <host>
  ./g8e operator stream <host...>
  ./g8e operator ssh-config

Testing:
  ./g8e test g8ee <path>
  ./g8e test g8ed <path>
  ./g8e test g8eo <path>

Config:
  ./g8e llm setup
  ./g8e llm show
  ./g8e data settings show
  ./g8e mcp config --client <name>

Security:
  ./g8e security validate
  ./g8e security certs generate

ALL COMMANDS

login - Authenticate and save session
  --api-key <key>, --device-token <token>

logout - Clear saved session

platform - Docker services (host)
  setup, settings, update
  start, stop, restart, status
  rebuild, reset, wipe, clean
  logs [service]

operator - Build and deploy
  init, build, build-all
  deploy <host>, stream <host>
  ssh-config, reauth

test - Run component tests
  g8ee, g8ed, g8eo
  Options: -j [N|auto]
  LLM flags: -p, -m, -a, -e, -k

security - Security tools (g8ep)
  validate, mtls-test
  certs generate/rotate/status/trust
  scan-licenses, passkeys
  rotate-internal-token

data - Data management (g8ep)
  users, operators, store
  settings, audit, device-links

  store:
    stats, network, find, wipe, get-setting
    kv list|get
    <collection> list|get

llm - LLM tooling (host)
  setup, restart, show
  get <key>, set <key=val>

mcp - MCP integration
  config, test, status

search - Web search (host)
  setup, disable

ssh - SSH credentials (host)
  setup

aws - AWS credentials (host)
  setup

demo - Fleet demo (host)
  Demo mode (pre-authenticated):
    init            Initialize demo mode: creates demo user and device link token
    start           One-command setup: platform setup + demo init + demo up + demo stream
  Fleet lifecycle:
    up              Build and start all 10 demo nodes + dashboard
    down            Stop all demo nodes
    status          Show container status for all demo nodes
    clean           Remove all demo containers and networks
  Operator deployment:
    deploy          Deploy operators via API download (DEVICE_TOKEN optional if demo mode initialized)
    stream          Deploy operators via SSH streaming (DEVICE_TOKEN optional if demo mode initialized)
    discover-hosts  List discovered demo fleet hosts
    operators       Show operator status across the fleet
    vanish          Remove all operators (zero trace cleanup)
  Inspection:
    health          Check Flask backend health on all nodes
    nginx-check     Check nginx status and HTTP response codes
    logs            Follow all container logs
    shell N=<nn>    Shell into a specific node (e.g., shell N=01)
    dashboard       Print fleet dashboard URL (http://localhost:3000)

  Examples:
    ./g8e demo start                                  # Full setup: platform + demo + operators (recommended)
    ./g8e demo init                                   # Initialize demo mode with pre-authenticated token
    ./g8e demo up                                     # Start the fleet
    ./g8e demo stream                                 # Deploy operators (uses stored token if demo mode initialized)
    ./g8e demo stream DEVICE_TOKEN=dlk_xxx            # Deploy operators with custom token
    ./g8e demo operators                              # Check operator status
    ./g8e demo shell N=06                             # Debug broken node
    ./g8e demo vanish                                 # Clean up operators
    ./g8e demo clean                                  # Remove everything

DETAILED HELP
  ./g8e operator --help
  ./g8e test --help
