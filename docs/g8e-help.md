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
  up, down, status, clean
  health, nginx-check, operators
  logs, shell N=<nn>
  deploy, stream
  discover-hosts, vanish, dashboard

DETAILED HELP
  ./g8e operator --help
  ./g8e test --help
