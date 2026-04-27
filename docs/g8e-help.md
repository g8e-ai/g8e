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
  ./g8e platform start --dev      # Start with hot-reload for g8ee
  ./g8e platform stop
  ./g8e platform status
  ./g8e platform rebuild
  ./g8e platform rebuild --dev    # Rebuild with hot-reload for g8ee
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
  --dev                    Enable development mode with hot-reload bind mounts for g8ee

operator - Build and deploy
  init, build, build-all
  deploy <host>, stream <host>
  ssh-config, reauth

test - Run component tests
  g8ee, g8ed, g8eo
  Options: --coverage, --pyright, --ruff, --e2e, -j [N|auto]

  LLM configuration (g8ee only):
    -p, --llm-provider <provider>      LLM provider (anthropic, openai, gemini, etc.)
    -m, --primary-model <model>        Primary model for grading
    -a, --assistant-model <model>     Assistant model to evaluate
    -e, --llm-endpoint-url <url>      Custom LLM endpoint URL
    -k, --llm-api-key <key>           LLM API key

  Web Search configuration (g8ee only):
    Set via environment variables: TEST_WEB_SEARCH_PROJECT_ID, TEST_WEB_SEARCH_ENGINE_ID, TEST_WEB_SEARCH_API_KEY

  Examples:
    ./g8e test g8ee tests/unit
    ./g8e test g8ee --coverage
    ./g8e test g8ee --pyright --ruff
    ./g8e test g8ee --e2e
    ./g8e test g8ee -j auto
    ./g8e test g8ee -p anthropic -m claude-3-5-sonnet -k <key> tests/unit
    ./g8e test g8ee -p openai -m gpt-4 -a gpt-3.5-turbo -k <key> --coverage

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
  Fleet lifecycle:
    up              Build and start all 10 demo nodes + dashboard
    down            Stop all demo nodes
    status          Show container status for all demo nodes
    clean           Remove all demo containers and networks
  Operator deployment:
    deploy [-d <token>]  Deploy operators via API download (requires device token)
    stream [-d <token>]  Deploy operators via SSH streaming (requires device token)
    discover-hosts  List discovered demo fleet hosts
    operators       Show operator status across the fleet
    vanish          Remove all operators (zero trace cleanup)
  Profile management:
    profile list             List available demo profiles
    profile create <name>    Create a new profile from current demo/
    profile switch <name>    Switch current demo/ to a profile
    profile delete <name>    Delete a profile
  Inspection:
    health          Check Flask backend health on all nodes
    nginx-check     Check nginx status and HTTP response codes
    logs            Follow all container logs
    shell N=<nn>    Shell into a specific node (e.g., shell N=01)
    dashboard       Print fleet dashboard URL (http://localhost:3000)

  Examples:
    ./g8e platform setup                              # Start the platform
    ./g8e demo up                                     # Start the fleet
    ./g8e demo stream -d dlk_xxx                      # Deploy operators with device token (shorthand)
    ./g8e demo deploy -d dlk_xxx                      # Deploy operators via API download (shorthand)
    ./g8e demo stream DEVICE_TOKEN=dlk_xxx            # Deploy operators with device token (full syntax)
    ./g8e demo deploy DEVICE_TOKEN=dlk_xxx            # Deploy operators via API download (full syntax)
    ./g8e demo operators                              # Check operator status
    ./g8e demo shell N=06                             # Debug broken node
    ./g8e demo vanish                                 # Clean up operators
    ./g8e demo clean                                  # Remove everything

evals - Real-operator evaluation fleet (host)
  up [-d <token>] [--nodes N]    Bring up eval fleet with N nodes (default: 3)
  down                           Tear down the eval fleet
  status                         Show status of eval nodes
  logs <node-name>               Show logs for a specific node
  run --gold-set <path> [-d <token>]  Run full eval suite against gold set

  Examples:
    ./g8e evals up -d dlk_xxx                          # Bring up 3 eval nodes
    ./g8e evals up -d dlk_xxx --nodes 5                # Bring up 5 eval nodes
    ./g8e evals status                                # Check fleet status
    ./g8e evals logs eval-node-01                      # View node logs
    ./g8e evals down                                   # Tear down fleet

DETAILED HELP
  ./g8e operator --help
  ./g8e test --help
