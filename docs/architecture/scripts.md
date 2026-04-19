# g8e Scripts

Organized scripts for managing the g8e platform.

## `./g8e` — Platform Management CLI

The `g8e` script in the project root is the single entry point for all platform operations. It requires only Docker — use it instead of calling underlying scripts directly.

**Location:** `g8e` (project root)

The `g8e` script is the single entry point for managing every aspect of the g8e platform. It requires only Docker — no Go, Python, or any other toolchain on the host. Commands that operate on Docker services run directly on the host; commands that require the internal toolchain (`security`, `data`) are transparently forwarded into the g8ep container via `docker exec`. Test commands route to dedicated test-runner containers.

### Bootstrap

No manual setup steps are required. On `docker compose up` (including Docker Desktop), g8ep starts and its entrypoint builds the operator binary natively if absent. All other services follow in dependency order.

The `./g8e` CLI provides the same experience via `./g8e platform start` and adds orchestration helpers for rebuilds, reset, and wipes.

### Usage

```bash
./g8e <command> [subcommand] [options]
./g8e --help
```

Running `./g8e` with no arguments or `--help` runs `g8e.operator do --help` inside g8ep, printing the Operator's command reference.

### Commands

#### platform
Manages the Docker Compose services. All subcommands run on the host.

```bash
./g8e platform build                    # Build images and restart services (alias: 'platform rebuild')
./g8e platform setup                    # Full first-time setup: build all images, start platform
./g8e platform settings                 # Show effective platform settings (requires platform running)
./g8e platform update                   # Pull latest changes (with confirmation) and rebuild
./g8e platform status                   # Show container status and component versions
./g8e platform start                    # Start all platform services
./g8e platform stop                     # Stop all platform services
./g8e platform restart                  # Restart all platform services (no rebuild)
./g8e platform rebuild                  # Rebuild images and restart services
./g8e platform rebuild g8ed             # Rebuild a single service: g8es | g8ee | g8ed | g8ep
./g8e platform reset                    # Wipe ALL data volumes and rebuild from scratch (destructive)
./g8e platform wipe                     # Clear app data from the database (preserves platform settings, SSL, LLM)
./g8e platform clean                    # Remove all managed Docker resources (containers, images, volumes, cache)
./g8e platform logs [service]           # Tail service logs with filtering options
```

| Subcommand | Delegates to | Notes |
|------------|-------------|-------|
| `setup` | `scripts/core/build.sh setup` | Full first-time setup: no-cache build of all images, start platform |
| `settings` | `scripts/data/manage-g8es.py settings show` (inside g8ep) | Displays effective non-secret platform settings from the live internal API |
| `update` | `scripts/core/build.sh rebuild` | Pulls latest from `origin/main` with confirmation, then rebuilds |
| `status` | `scripts/core/build.sh status` | Show container status and component versions |
| `start` | `scripts/core/build.sh up` | |
| `stop` | `scripts/core/build.sh down` | |
| `restart` | `scripts/core/build.sh restart` | |
| `rebuild` | `scripts/core/build.sh rebuild` | Alias: `build`. Accepts optional component names: `g8es g8ee g8ed g8ep` |
| `reset` | `scripts/core/build.sh reset` | Wipe ALL data volumes and rebuild from scratch (destructive) |
| `wipe` | `scripts/core/build.sh wipe` | Clear app data from the database (preserves platform settings, SSL, LLM) |
| `clean` | `scripts/core/build.sh clean` | Remove all managed Docker resources (containers, images, volumes, cache) |
| `logs` | `scripts/core/logs.sh` | Supports filtering by level, pattern, time; follows or tail |

#### operator
Manages the Operator binary build and deployment.

```bash
./g8e operator init                          # Build the operator binary inside g8eo-test-runner
./g8e operator build                         # Rebuild the operator binary inside g8eo-test-runner
./g8e operator build-all                     # Build all operator architectures with compression (for distribution)
./g8e operator deploy <user@host>            # Copy operator to remote host via scp
./g8e operator stream <host...>              # Stream-inject operator to one or more remote hosts
./g8e operator ssh-config                    # Configure ~/.ssh/config for high-concurrency streaming
./g8e operator reauth --user-id <id>         # Kill and relaunch the g8ep operator for a user
```

| Subcommand | Delegates to | Notes |
|------------|-------------|-------|
| `init` | `docker exec g8eo-test-runner ...` | Builds operator binary natively inside g8eo-test-runner |
| `build` | `scripts/core/build.sh operator-build` | Rebuild of operator binary inside g8eo-test-runner |
| `build-all` | `scripts/core/build.sh operator-build-all` | Build all operator architectures with compression |
| `deploy` | `scp` + optional `ssh` | Supports `--arch`, `--dest`, `--endpoint`, `--device-token`, `--key`, `--no-git` |
| `stream` | `docker exec g8ep /home/g8e/g8e.operator stream` | Zero local disk footprint. Supports `--arch`, `--hosts`, `--concurrency`, `--timeout`, `--endpoint`, `--device-token`, `--key`, `--no-git`, `--ssh-config` |
| `ssh-config` | `scripts/tools/setup-ssh.sh` | Configures multiplexing. Supports `--print`, `--force` |
| `reauth` | Internal API | Requires `--user-id <id>` or `--email <email>` |

#### test
Runs tests for platform components in dedicated test-runner containers.

```bash
./g8e test g8ee                     # AI engine (Python/pytest)
./g8e test g8ed                     # Dashboard (Node/Vitest)
./g8e test g8eo                     # Operator (Go)
./g8e test g8ee --coverage          # Generate coverage report
./g8e test g8ee --llm-provider gemini # Run with a specific LLM provider
./g8e test g8ee --primary-model <m> # Override the LLM model for the run
./g8e test g8eo -- TestFoo          # Pass extra args to the underlying test runner
```

#### security
Security validation and certificate management. Runs inside g8ep.

```bash
./g8e security validate              # Validate platform security (volumes, env vars, tokens)
./g8e security certs                # Show TLS certificate status (default)
./g8e security certs generate       # Generate CA + server certificates
./g8e security certs rotate         # Rotate/regenerate existing certificates
./g8e security certs status         # Show certificate status and expiry
./g8e security certs trust          # Install CA into host OS store (host-side)
./g8e security mtls-test            # Run mTLS connectivity tests
./g8e security passkeys             # Manage user passkey credentials
./g8e security rotate-internal-token # Rotate the X-Internal-Auth shared secret across all components
```

#### data
Data management. All subcommands route through `manage-g8es.py` inside g8ep.

```bash
./g8e data users list                        # List users
./g8e data users create --email <e> --name <n>
./g8e data operators list --email <e>        # List operators for a user
./g8e data operators get --id <operator-id>  # Get full operator details
./g8e data operators init-slots --email <e>  # Initialize operator slots for a user
./g8e data operators refresh-key --id <id>   # Refresh operator API key
./g8e data operators get-key --id <id>       # Fetch current operator API key
./g8e data operators reset --id <id>         # Reset operator to fresh AVAILABLE state
./g8e data settings show --section llm       # Show LLM settings
./g8e data settings set llm_model=gemma3:4b  # Write settings
./g8e data store stats                       # g8es statistics
./g8e data store <collection>                # List documents (e.g., operators, sessions, users)
./g8e data audit stats                       # LFAA audit vault stats
./g8e data audit sessions                    # List LFAA sessions
./g8e data device-links list --email <e>     # List device link tokens
```

#### llm
Local LLM container management. Runs on the host.

```bash
./g8e llm setup                              # Interactive LLM provider setup
./g8e llm restart                            # Force-recreate local LLM container
./g8e llm show                               # Show current LLM settings
./g8e llm get <key>                          # Read a single LLM setting
./g8e llm set <key=value> [...]              # Write one or more LLM settings
```

#### mcp
MCP client integration for external AI tools. Runs inside g8ep.

```bash
./g8e mcp config --client claude-code --email user@example.com  # Generate Claude Code config
./g8e mcp config --client windsurf --email user@example.com     # Generate Windsurf config
./g8e mcp config --client cursor --email user@example.com       # Generate Cursor config
./g8e mcp config --client generic --email user@example.com      # Generic MCP client config
./g8e mcp test --email user@example.com                         # Test MCP endpoint connectivity
./g8e mcp status                                                # Show endpoint info
```

#### search
Vertex AI Search configuration for the search_web AI tool. Runs on the host.

```bash
./g8e search setup                           # Configure Vertex AI Search (interactive)
./g8e search disable                         # Remove web search configuration
```

#### ssh
SSH configuration for operator streaming. Runs on the host.

```bash
./g8e ssh setup                              # Mount SSH directory into g8ep
```

#### aws
AWS credentials configuration for AI tools. Runs on the host.

```bash
./g8e aws setup                              # Mount AWS credentials directory into g8ep
```

#### demo
Manage the broken-fleet demo. Runs on the host.

```bash
./g8e demo up                       # Build and start demo nodes
./g8e demo down                     # Stop all nodes
./g8e demo status                   # Show container status
./g8e demo clean                    # Remove everything (containers, images, volumes)
./g8e demo logs                     # Follow all container logs
./g8e demo dashboard                # Print dashboard URL
```

---

## Directory Structure

```
scripts/
  core/           # Platform build scripts
    build.sh      #   Docker Compose orchestration
    logs.sh       #   Log aggregation and filtering
    setup.sh      #   First-time setup
  data/           # Data management (unified via manage-g8es.py dispatcher)
    _lib.py       #   Shared auth, HTTP clients, display helpers
    manage-g8es.py  #   Entry point — routes to resource scripts
    manage-store.py  #   g8es document store & KV queries
    manage-users.py  #   User CRUD
    manage-settings.py  # Platform settings
    manage-operators.py # Operator management
    manage-device-links.py  # Device link tokens
    manage-lfaa.py   #   LFAA audit vault (SQLite)
    manage-mcp.py    #   MCP client integration (config, test, status)
  security/       # TLS certificates and security validation
    manage-ssl.sh      #   Certificate lifecycle
    mtls-test.sh      #   mTLS connectivity verification
    validate-platform-security.sh  # Security validation
    manage-passkeys.py #   Passkey credential management
    trust-ca.ps1       #   Windows CA trust installation
  testing/        # Test runner and supporting tools
    run_tests.sh  #   Component test execution
  tools/          # Developer utilities
    setup-llm.sh      #   LLM provider configuration
    setup-ssh.sh      #   SSH configuration for operator streaming
    setup-aws.sh      #   AWS credentials mount
    setup-search.sh   #   Vertex AI Search configuration
```

---

## Log Aggregation (`logs.sh`)

**Location:** `scripts/core/logs.sh`

Aggregates and filters logs across all platform services in time order.

### Usage

```bash
./g8e platform logs [options] [service...]
```

### Options

| Option | Description |
|--------|-------------|
| `-g, --grep <pattern>` | Include lines matching pattern (case-insensitive regex) |
| `-v, --invert <pattern>` | Exclude lines matching pattern |
| `-l, --level <level>` | Filter by log level: error, warn, info, debug |
| `-s, --since <duration>` | Show logs since duration (e.g. 5m, 1h, 30s) or timestamp |
| `-n, --tail <N>` | Lines from end per service (default: 200; use 'all' for all) |
| `-f, --follow` | Stream new log lines (default: off) |
| `--all` | Include g8ep/operator (default: core only) |

### Examples

```bash
./g8e platform logs                          # last 200 lines, all core services
./g8e platform logs --level error            # errors only
./g8e platform logs --level warn --follow    # stream warnings+
./g8e platform logs --grep 'operator|investigation'
./g8e platform logs --since 5m               # last 5 minutes
./g8e platform logs g8ee g8ed --tail 50
./g8e platform logs g8ep                     # operator process output
```

---

## Core Build Script (`build.sh`)

**Location:** `scripts/core/build.sh`

Builds and runs the local g8e environment. Manages Docker Compose services in dependency order, waits for health checks, optionally wipes data volumes, handles parallel rebuilds, and stamps version into `VERSION` files on rebuild.

### Usage

```bash
./scripts/core/build.sh <command> [options]
```

### Subcommands

| Command | Description |
|---------|-------------|
| `status` | Show container status and component versions |
| `up [component...]` | Start managed services without building. Default: `g8es g8ee g8ed g8ep`. |
| `down` | Stop managed containers (`g8es`, `g8ee`, `g8ed`, `g8ep`) |
| `restart` | Restart all containers (no rebuild) |
| `rebuild [component...]` | Rebuild images and restart services. Default: `g8es g8ee g8ed g8ep` |
| `reset` | Wipe DB data volumes + rebuild images from scratch (destructive) |
| `wipe` | Clear app data from the database (preserves platform settings, SSL, LLM) |
| `clean` | Remove all managed Docker resources (containers, images, volumes, cache) |
| `setup` | Full first-time setup: build all images, start platform |
| `operator-build` | Build linux/amd64 operator binary inside g8eo-test-runner |
| `operator-build-all` | Build all operator architectures with compression |

---

## Testing (`run_tests.sh`)

**Location:** `scripts/testing/run_tests.sh`

Runs tests for g8e components in dedicated test-runner containers. Infrastructure must already be running.

### Usage

```bash
./scripts/testing/run_tests.sh [COMPONENT] [OPTIONS] [-- EXTRA_ARGS]
```

### Components

| Component | Test Framework | Test Runner Container | What It Tests |
|-----------|---------------|----------------------|--------------|
| `g8ee` | pytest | `g8ee-test-runner` | g8ee Python service |
| `g8ed` | vitest / npm test | `g8ed-test-runner` | g8ed Node.js service |
| `g8eo` | `gotestsum` | `g8eo-test-runner` | g8eo Go binary |
| `all` | All of the above | All test runners | Full suite (not directly supported via ./g8e) |

### Options

| Option | Description |
|--------|-------------|
| `--coverage` | Generate coverage reports |
| `--pyright` | Run pyright strict gate (g8ee only) |
| `--e2e` | Run E2E operator lifecycle tests (g8ee only) |

---

## Data Management

**Entry point:** `scripts/data/manage-g8es.py`

All data operations route through a single dispatcher (`manage-g8es.py`) that delegates to individual resource scripts. Each resource script can also run standalone. Shared HTTP clients, authentication, and display helpers live in `scripts/data/_lib.py`.

```
scripts/data/
  manage-g8es.py        # Entry point — dispatches to resource scripts
  _lib.py                # Shared: auth, HTTP clients (g8es + g8ed), display helpers
  manage-store.py        # g8es document store & KV queries
  manage-users.py        # User CRUD via g8ed internal API
  manage-settings.py     # Platform settings read/write via g8ed internal API
  manage-operators.py    # Operator management via g8ed internal API
  manage-device-links.py # Device link token management via g8ed internal API
  manage-lfaa.py         # LFAA audit vault queries (local SQLite)
  manage-mcp.py          # MCP client integration (config generation, endpoint testing)
```

### Platform Settings (`settings`)

Read and display effective platform settings via the g8ed internal HTTP API. Secret values (API keys, tokens) are never returned. Shows the live effective value for each setting — env-locked entries are marked with `[env]`.

```bash
./g8e platform settings                          # Show all settings
./g8e platform settings --section llm            # Filter by section: general, llm, search, security
./g8e data settings show                         # Same as above
./g8e data settings get llm_provider             # Read a single setting value
./g8e data settings set llm_model=gemma3:4b      # Write settings to the DB
./g8e data settings export --section llm         # Export as clean JSON
```

### User Management (`users`)

User CRUD, role management, and statistics.

```bash
./g8e data users list
./g8e data users create --email user@example.com --name "John Doe"
./g8e data users update-role --id USER_ID --role admin
./g8e data users delete --id USER_ID
./g8e data users stats
```

### Operator Management (`operators`)

Manage operator documents via the g8ed internal HTTP API. All operations target the g8es document store through g8ed — no direct DB access.

```bash
./g8e data operators list --user-id USER_ID
./g8e data operators list --email user@example.com
./g8e data operators list --email user@example.com --all   # Include terminated
./g8e data operators get --id OPERATOR_ID
./g8e data operators init-slots --user-id USER_ID
./g8e data operators init-slots --email user@example.com
./g8e data operators refresh-key --id OPERATOR_ID          # Prompts for confirmation
./g8e data operators refresh-key --id OPERATOR_ID --force
./g8e data operators get-key --id OPERATOR_ID
./g8e data operators reset --id OPERATOR_ID                # Prompts for confirmation
./g8e data operators reset --id OPERATOR_ID --force
```

**`refresh-key`** terminates the old operator document (invalidating its API key and revoking its cert) and creates a new document at the same slot number with a fresh API key. This is the same operation g8ed performs when a user clicks "Refresh Key" in the UI.

**`reset`** deletes and recreates the operator document with default values, preserving the existing API key and slot number. Session state is cleared. Used for demo resets.

### Device Link Management (`device-links`)

Generate, list, and revoke device link tokens.

```bash
./g8e data device-links list --email user@example.com
./g8e data device-links create --email user@example.com --name "prod-fleet" --max-uses 50
./g8e data device-links revoke --token dlk_...
./g8e data device-links delete --token dlk_...
```

### g8es Document Store (`store`)

Query the g8es document store and KV store via the HTTP API. Runs inside g8ep and communicates with g8es directly.

```bash
./g8e data store stats                                    # g8es statistics
./g8e data store operators                                # List operators collection
./g8e data store doc --collection operators --id <id>     # Get a single document
./g8e data store find --collection operators --field status --value active
./g8e data store network                                  # Operator network details
./g8e data store kv --pattern "g8e:session:*"              # List KV keys
./g8e data store kv-get --key "g8e:session:web:session_123"
./g8e data store wipe --dry-run                           # Clear app data (preserves settings)
./g8e data store get-setting llm_model                    # Read a raw platform setting
```

### LFAA Audit Vault (`audit`)

Query the operator's Local-First Audit Architecture (LFAA) vault (SQLite).
Requires `--container NAME` (for operator containers with local storage) or `--db-path PATH` or `--volume NAME`.

```bash
./g8e data audit --container operator-test-1 stats
./g8e data audit --container operator-test-1 sessions
./g8e data audit --container operator-test-1 events --session <id>
./g8e data audit --container operator-test-1 export --session <id> --out audit.json
```

---

## Security

### Platform Security Validation (`validate-platform-security.sh`)
**Location:** `scripts/security/validate-platform-security.sh`

Validates platform security configuration by checking:
- TLS certificate files exist in g8es
- Volume mounts are correct (g8ed, g8ee have access to g8es certs and tokens)
- Environment variables match mounted files (INTERNAL_AUTH_TOKEN, SESSION_ENCRYPTION_KEY)

```bash
./g8e security validate
```

### TLS Certificate Management (`manage-ssl.sh`)
**Location:** `scripts/security/manage-ssl.sh`

Manages the platform TLS certificates owned by g8es. g8es generates the CA and server certificates automatically on first start — this script orchestrates lifecycle via docker commands.

```bash
./g8e security certs generate   # Ensure certs exist (idempotent — no-op if already present)
./g8e security certs rotate     # Force-regenerate: wipe /data/ssl/ and restart g8es
./g8e security certs status     # Show cert expiry, subject, and SANs
./g8e security certs trust      # Install CA into host OS trust store
```

After `rotate`, run `./g8e platform rebuild` to re-embed the new CA into the operator binary.

### Passkey Management (`manage-passkeys.py`)
**Location:** `scripts/security/manage-passkeys.py`

Manage FIDO2/WebAuthn passkey credentials via the g8ed internal HTTP API.

```bash
./g8e security passkeys list --email user@example.com
./g8e security passkeys list --id USER_ID
./g8e security passkeys revoke --id USER_ID --credential CRED_ID
./g8e security passkeys revoke --email user@example.com --credential CRED_ID --force
./g8e security passkeys revoke-all --email user@example.com
./g8e security passkeys revoke-all --id USER_ID --force
./g8e security passkeys reset --email user@example.com
./g8e security passkeys reset --id USER_ID --force
```

**`reset`** removes all passkey credentials for a user. The user will be prompted to register a new passkey on next login. Existing sessions expire naturally.

**`revoke`** removes a specific credential by ID.

**`revoke-all`** removes all credentials, locking the user out until a new passkey is registered.

### Internal Token Rotation

Rotate the internal auth token across all components.

```bash
./g8e security rotate-internal-token
```

---

### mTLS Verification (`mtls-test.sh`)
**Location:** `scripts/security/trust-ca.ps1`

One-shot PowerShell script for Windows users. Removes any previously installed g8e CA cert, fetches the new one from the platform via SSH, and installs it into `LocalMachine\Root` — all in one step.

Run this in an **Administrator PowerShell** prompt after each platform rebuild or cert rotation:

```powershell
.\trust-ca.ps1 -Server admin@10.0.0.2
```

The `-Server` parameter accepts a `user@host`, bare hostname, or SSH config alias.

**Alternative:** Use the platform's auto-detect trust endpoint instead of this script:
```powershell
irm http://<host>/trust | iex
```

---

### mTLS Verification (`mtls-test.sh`)
**Location:** `scripts/security/mtls-test.sh`

Verifies mTLS configuration for g8eo ↔ g8es communication.

```bash
./scripts/security/mtls-test.sh
```

---

## LLM Setup Wizard (`setup-llm.sh`)

**Location:** `scripts/tools/setup-llm.sh`

Interactive wizard to configure LLM providers (Ollama, OpenAI, Anthropic, Gemini, vLLM).

> **Recommended: Google Gemini 3.1.** The platform was designed around Gemini best practices and the Gemini integration is the most robust and extensively tested. Other providers are supported but are not part of the standard test pipeline.

```bash
./g8e llm setup
./g8e llm show
./g8e llm get llm_model
./g8e llm set llm_model=gemma3:4b
./g8e llm restart
```

---

## SSH Configuration (`setup-ssh.sh`)

**Location:** `scripts/tools/setup-ssh.sh`

Configures SSH directory mounting for operator streaming. The ssh-config subcommand configures ~/.ssh/config for high-concurrency operator streaming with multiplexing.

```bash
./g8e ssh setup                              # Mount SSH directory into g8ep
./g8e operator ssh-config                    # Configure ~/.ssh/config for streaming
./g8e operator ssh-config --print           # Print the stanza without writing
./g8e operator ssh-config --force            # Replace existing stanza
```

The ssh-config subcommand creates:
- ~/.ssh/sockets/ directory for ControlMaster sockets
- Multiplexing stanza in ~/.ssh/config for connection reuse
- Keep-alive settings and compression for efficient streaming

---

## AWS Credentials Setup (`setup-aws.sh`)

**Location:** `scripts/tools/setup-aws.sh`

Mounts AWS credentials into g8ep so the operator can interact with AWS services.

```bash
./g8e aws setup                              # Interactive setup
./g8e aws setup --aws-dir /custom/path        # Non-interactive
```

The mounted directory is configured in docker-compose.yml as ${HOME}/.aws by default. Update the volume mount in docker-compose.yml to use a custom path.

---

## Vertex AI Search Setup (`setup-search.sh`)

**Location:** `scripts/tools/setup-search.sh`

Interactive Vertex AI Search configuration for the search_web AI tool. Enables AI to search the web during investigations using a Vertex AI Search (Discovery Engine) app.

```bash
./g8e search setup                           # Interactive setup
./g8e search setup --project-id PROJECT --engine-id ENGINE --api-key KEY
./g8e search disable                         # Remove web search configuration
```

**Prerequisites (one-time GCP setup):**
1. Enable the Discovery Engine API in your GCP project
2. Create a Website data store and index your domains
3. Create a Search App connected to that data store
4. Create an API key restricted to the Discovery Engine API

The script validates the API key against Vertex AI Search before writing configuration to the database.

---

## Common Patterns

All scripts resolve `PROJECT_ROOT` consistently:

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
