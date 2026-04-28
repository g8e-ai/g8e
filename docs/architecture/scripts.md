---
title: Scripts
parent: Architecture
---

# g8e Scripts

The scripts directory contains the operational tooling for the g8e platform. All scripts are designed to be invoked through the unified `./g8e` CLI, which abstracts away Docker complexity and provides a single interface for platform operations.

## Architecture Overview

The scripts layer follows a clear separation of concerns:

- **Host-side operations** (`platform`, `operator`, `test`): Run directly on the host machine, managing Docker Compose services and test execution
- **Container-side operations** (`security`, `data`, `mcp`): Forwarded into the g8ep container via `docker exec`, where they have access to the full Python toolchain and internal APIs
- **Test runners**: Dedicated containers (g8ee-test-runner, g8ed-test-runner, g8eo-test-runner) with isolated environments for component testing

### The `./g8e` CLI Entry Point

The `g8e` script at the project root is the single entry point for all platform operations. It requires only Docker on the host — no Go, Python, or other toolchains are needed locally.

**Why this design:** By routing all operations through a single CLI, the platform can:
- Validate authentication state before container-side operations
- Automatically ensure required containers are running
- Provide consistent error handling and logging
- Abstract Docker Compose complexity from operators

**Execution flow:**
1. Parse command and flags (including authentication)
2. For host commands: Execute directly with Docker Compose
3. For container commands: Ensure g8ep is running, then `docker exec` the script
4. For test commands: Route to appropriate test-runner container with environment variables

### Bootstrap

No manual setup steps are required. On `docker compose up` (including Docker Desktop), g8ep starts and its entrypoint builds the operator binary natively if absent. All other services follow in dependency order.

The `./g8e` CLI provides the same experience via `./g8e platform start` and adds orchestration helpers for rebuilds, reset, and wipes.

### Core Commands

#### platform
Manages Docker Compose services and platform lifecycle. All operations run on the host.

**Key operations:**
- `start` / `stop` / `restart`: Basic service control without rebuilding
- `rebuild [component]`: Rebuild images with layer cache, preserving volumes
- `reset`: Destructive — wipes DB data volumes and rebuilds from scratch (SSL certs preserved)
- `wipe`: Clears app data from database while preserving platform settings, SSL, and LLM configuration
- `clean`: Removes all managed Docker resources (containers, images, volumes, networks)
- `logs`: Aggregates and filters logs across all services with time-ordered output

**Why separate `reset` and `wipe`:**
- `reset` is for full environment reset (e.g., after schema changes) — destroys and recreates volumes
- `wipe` is for data cleanup (e.g., demo resets) — clears collections via HTTP API, preserving infrastructure

#### operator
Builds and deploys the g8eo operator binary to remote hosts.

**Key operations:**
- `build` / `build-all`: Compiles operator binary in g8eo-test-runner container
- `deploy <user@host>`: Copies binary via scp and optionally starts it remotely
- `stream <hosts...>`: Zero-footprint streaming injection via SSH multiplexing
- `ssh-config`: Configures ~/.ssh/config for high-concurrency streaming
- `reauth`: Forces operator re-authentication for a specific user

**Why streaming vs deploy:**
- `deploy` is for one-off installations to a single host
- `stream` is for fleet-wide updates with SSH multiplexing, supporting concurrent deployment to hundreds of hosts

#### test
Runs component tests in isolated test-runner containers.

**Components:**
- `g8ee`: Python pytest with optional coverage, pyright, ruff, and E2E tests
- `g8ed`: Node.js Vitest with coverage support
- `g8eo`: Go tests via gotestsum with race detection and coverage

**Test isolation:** Each component has its own test-runner container with the exact toolchain version required, preventing host environment conflicts.

#### security
Validates platform security configuration and manages certificates. Runs inside g8ep.

**Key operations:**
- `validate`: Checks TLS certificates, volume mounts, and environment variable consistency
- `certs generate/rotate/status`: Manages the platform's self-signed CA and server certificates
- `passkeys`: Manages FIDO2/WebAuthn credentials via g8ed internal API
- `rotate-internal-token`: Rotates the X-Internal-Auth shared secret across all components

**Security invariant:** The g8es-ssl volume is never wiped by `reset` or `clean` — certificates persist across environment resets to maintain trust continuity.

#### data
Data management operations routed through `manage-g8es.py` dispatcher inside g8ep.

**Resource modules:**
- `store`: Direct g8es document store and KV queries
- `users`: User CRUD via g8ed internal API
- `operators`: Operator document management via g8ed internal API
- `settings`: Platform settings read/write via g8ed internal API
- `device-links`: Device link token management
- `audit`: LFAA audit vault queries (SQLite)
- `mcp`: MCP client configuration and endpoint testing
- `reputation`: Reputation state seeding and commitment chain repair

**Why dispatcher pattern:** Centralizes authentication, HTTP client configuration, and error handling. Each resource module can also run standalone for debugging.

#### llm
Configures LLM provider settings. Runs on the host.

**Why host-side:** LLM configuration is written to environment variables and docker-compose.yml, which are host-managed. The platform reads these at startup.

#### mcp
Generates MCP client configurations for external AI tools (Claude Code, Windsurf, Cursor). Runs inside g8ep.

**Why container-side:** Requires access to g8ed internal API to generate authenticated configuration with valid session tokens.

#### search / ssh / aws / demo
Utility commands for external service integration and demo environment management.

---

## Directory Structure

```
scripts/
  core/           # Platform lifecycle orchestration
    build.sh      #   Docker Compose service management (up, rebuild, reset, wipe, clean)
    logs.sh       #   Log aggregation with time-ordered filtering
    setup.sh      #   Pass-through to LLM provider configuration
  data/           # Data management (unified via manage-g8es.py dispatcher)
    _lib.py       #   Shared auth, HTTP clients, display helpers
    manage-g8es.py #   Entry point — routes to resource scripts
    manage-store.py #   g8es document store & KV queries
    manage-users.py #   User CRUD via g8ed internal API
    manage-settings.py # Platform settings via g8ed internal API
    manage-operators.py # Operator management via g8ed internal API
    manage-device-links.py # Device link token management
    manage-lfaa.py #   LFAA audit vault queries (local SQLite)
    manage-mcp.py  #   MCP client integration (config, test, status)
    manage-reputation.py # Reputation state seeding and commitment chain repair
    seed-reputation-state.py # Bootstrap script for initial reputation state
  security/       # TLS certificates and security validation
    manage-ssl.sh #   Certificate lifecycle (generate, rotate, status)
    mtls-test.sh #   mTLS connectivity verification
    validate-platform-security.sh # Security validation
    manage-passkeys.py # Passkey credential management
    trust-ca.ps1  #   Windows CA trust installation
  testing/        # Test runner and supporting tools
    run_tests.sh  #   Component test execution
    gen_ledger_hash_fixtures.py # Test fixture generation
    measure_prefix_cache.sh # Performance measurement
  tools/          # Developer utilities
    setup-llm.sh  #   LLM provider configuration
    setup-ssh.sh  #   SSH configuration for operator streaming
    setup-aws.sh  #   AWS credentials mount
    setup-search.sh # Vertex AI Search configuration
```

---

## Core Subsystems

### Platform Lifecycle (`build.sh`)

**Location:** `scripts/core/build.sh`

Orchestrates Docker Compose services with dependency-aware health checks. This is the backbone of all `./g8e platform` commands.

**Key responsibilities:**
- Service startup in dependency order (g8es → g8ee → g8ed → g8ep)
- Health check waiting with live log streaming during startup
- Volume management with clear separation between data and SSL
- Version stamping and parallel rebuild support

**Volume invariants:**
- `g8es-data`, `g8ee-data`, `g8ed-data`: Wiped by `reset`, preserved by `wipe`
- `g8es-ssl`: NEVER wiped — certificates persist across all operations to maintain trust
- `g8ed-node-modules`: Wiped by `reset` to ensure clean dependency state

**Why health checks:** Services report readiness via Docker health checks rather than simple container running status. This prevents cascading failures when a service starts but isn't actually ready to accept connections.

### Log Aggregation (`logs.sh`)

**Location:** `scripts/core/logs.sh`

Aggregates logs from all services in time order with filtering capabilities.

**Why time ordering:** When debugging multi-service interactions, seeing events across services in chronological order is critical. The script interlaces log lines by timestamp to reconstruct the actual event sequence.

**Key features:**
- Level filtering (error, warn, info, debug)
- Pattern matching with regex support
- Time-based filtering (since duration or timestamp)
- Service-specific tailing
- Follow mode for live monitoring

### Test Execution (`run_tests.sh`)

**Location:** `scripts/testing/run_tests.sh`

Runs inside dedicated test-runner containers, never on the host. The `./g8e` CLI handles container selection and environment variable injection.

**Test isolation strategy:**
- Each component has its own test-runner container with exact toolchain versions
- CA certificates and platform secrets are mounted from g8es volumes
- LLM and web search configuration is injected via environment variables for flexible testing

**Why containerized testing:** Prevents host environment conflicts (Python version mismatches, Node version drift) and ensures reproducible test environments across developer machines.

### Data Management Dispatcher (`manage-g8es.py`)

**Location:** `scripts/data/manage-g8es.py`

Single entry point for all data operations. Dispatches to resource-specific modules based on the first argument.

**Why dispatcher pattern:**
- Centralizes authentication and HTTP client configuration
- Provides consistent error handling and output formatting
- Allows each resource module to run standalone for debugging
- Reduces code duplication across resource scripts

**Shared library (`_lib.py`):** Provides authenticated HTTP clients for both g8es (document store) and g8ed (internal API), display helpers, and credential loading.

### Reputation Management (`manage-reputation.py`)

**Location:** `scripts/data/manage-reputation.py`

Manages the Tribunal ensemble's reputation state and commitment chain.

**Key operations:**
- `seed`: Initializes reputation state for all agent personas (axiom, concord, variance, pragma, nemesis, sage, triage, auditor)
- `repair`: Verifies and fixes broken commitment chain links by re-signing with the auditor HMAC key

**Why separate seed script:** The `seed-reputation-state.py` script is called by `manage-reputation.py seed` but can also run independently. This allows for idempotent seeding during development without the full dispatcher overhead.

**Commitment chain invariant:** Each commitment contains a `prev_root` field linking to the previous commitment's Merkle root. The `repair` command detects breaks in this chain and re-signs from the first break point forward.

### Security Validation

**Location:** `scripts/security/validate-platform-security.sh`

Validates that the platform's security configuration is correct by checking:
- TLS certificate files exist in g8es
- Volume mounts are correct (g8ed and g8ee have access to g8es certs and tokens)
- Environment variables match mounted file contents (INTERNAL_AUTH_TOKEN, SESSION_ENCRYPTION_KEY)

**Why read from /proc/1/environ:** Docker exec starts a new shell session without exported environment variables. Reading from the main process's environment ensures we validate the actual runtime configuration, not a shell session state.

### TLS Certificate Lifecycle

**Location:** `scripts/security/manage-ssl.sh`

Orchestrates certificate generation and rotation via docker commands. g8es generates the CA and server certificates automatically on first start — this script provides user-friendly lifecycle management.

**Certificate persistence:** Certificates live in the `g8es-ssl` volume, which is never wiped by `reset` or `clean`. This ensures trust continuity across environment resets.

**Rotation workflow:**
1. `rotate` wipes `/data/ssl/` inside g8es
2. g8es restarts and generates new certificates
3. `./g8e platform rebuild` re-embeds the new CA into operator binaries
4. Hosts must re-trust the new CA via the `/trust` endpoint

### External Service Integration

**LLM Setup (`setup-llm.sh`):** Interactive wizard for configuring LLM providers. Writes configuration to docker-compose.yml environment variables, which are read at container startup.

**SSH Setup (`setup-ssh.sh`):** Mounts the host's SSH directory into g8ep and configures ~/.ssh/config for multiplexing. This is required for operator streaming to remote hosts.

**AWS Setup (`setup-aws.sh`):** Mounts AWS credentials into g8ep so the operator can interact with AWS services (e.g., for AI tools that need S3 or EC2 access).

**Vertex AI Search Setup (`setup-search.sh`):** Configures the search_web AI tool with Vertex AI Search credentials. Validates the API key before writing to the database.

---

## Common Patterns

### Path Resolution

All scripts resolve `PROJECT_ROOT` consistently to work from any invocation location:

**Bash:**
```bash
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"
```

**Python:**
```python
from pathlib import Path
PROJECT_ROOT = Path(__file__).parent.parent.parent.resolve()
```

### Container Health Waiting

The `build.sh` script uses a pattern for waiting on service health with live log streaming:

```bash
_wait_healthy() {
    local name="$1" timeout_s="$2" interval="${3:-1}"
    # Start log tailing in background
    docker logs -f "$name" 2>&1 | grep --line-buffered -E "started|error|ERROR" | sed "s/^/    [$name] /" &
    local log_pid=$!
    trap "kill $log_pid 2>/dev/null || true" RETURN
    
    until [ "$(docker inspect --format='{{.State.Health.Status}}' "$name")" = "healthy" ]; do
        # timeout logic
        sleep "$interval"
    done
    
    kill $log_pid 2>/dev/null || true
}
```

**Why this pattern:** Provides visibility into startup progress while waiting for health checks, making debugging slow or failing services easier.
