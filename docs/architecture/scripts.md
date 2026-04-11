# g8e Scripts

Organized scripts for managing the g8e platform.

## `./g8e` — Platform Management CLI

The `g8e` script in the project root is the single entry point for all platform operations. It requires only Docker — use it instead of calling underlying scripts directly.

**Location:** `g8e` (project root)

The `g8e` script is the single entry point for managing every aspect of the g8e platform. It requires only Docker — no Go, Python, or any other toolchain on the host. Commands that operate on Docker services run directly on the host; commands that require the internal toolchain (`test`, `security`, `data`) are transparently forwarded into the g8e-pod container via `docker exec`.

### Bootstrap

No manual setup steps are required. On `docker compose up` (including Docker Desktop), g8e-pod starts and its entrypoint builds the operator binary natively if absent. All other services follow in dependency order.

The `./g8e` CLI provides the same experience via `./g8e platform start` and adds orchestration helpers for rebuilds, reset, and wipes.

### Usage

```bash
./g8e <command> [subcommand] [options]
./g8e --help
```

Running `./g8e` with no arguments or `--help` runs `g8e.operator do --help` inside g8e-pod, printing the Operator's command reference.

### Commands

#### platform
Manages the Docker Compose services. All subcommands run on the host.

```bash
./g8e platform build                    # Build all images and start the platform (alias: drop)
./g8e platform settings                 # Show effective platform settings (requires platform running)
./g8e platform update                   # Pull latest changes (with confirmation) and rebuild
./g8e platform status                   # Show container status and versions
./g8e platform start                    # Start the platform
./g8e platform stop                     # Stop all containers (data preserved)
./g8e platform restart                  # Restart all containers (no rebuild)
./g8e platform rebuild                  # No-cache rebuild of all services + restart
./g8e platform rebuild vsod             # Rebuild a single service: vsodb | vse | vsod | g8e-pod
./g8e platform wipe                     # Wipe all data volumes and restart fresh
./g8e platform clean                    # Full Docker cleanup: containers, images, volumes, networks (this project only)
./g8e platform logs [service]           # Tail service logs
```

| Subcommand | Delegates to | Notes |
|------------|-------------|-------|
| `settings` | `scripts/data/manage-vsodb.py settings show` (inside g8e-pod) | Displays effective non-secret platform settings from the live internal API |
| `update` | `scripts/core/build.sh rebuild` | Pulls latest from `origin/main` with confirmation, then rebuilds |
| `status` | `scripts/core/build.sh status` | |
| `start` | `scripts/core/build.sh up` | |
| `stop` | `scripts/core/build.sh down` | |
| `restart` | `scripts/core/build.sh restart` | |
| `rebuild` | `scripts/core/build.sh rebuild` | Alias: `build`, `drop`. Accepts optional component names: `vsodb vse vsod g8e-pod` |
| `reset` | `scripts/core/build.sh reset` | Wipe ALL data volumes and rebuild from scratch (destructive) |
| `wipe` | `scripts/core/build.sh wipe` | Clear app data from the database (preserves platform settings, SSL, LLM) |
| `clean` | `scripts/core/build.sh clean` | Remove all managed resources scoped to this project |
| `logs` | `scripts/core/logs.sh` | |

#### operator
Manages the Operator binary build and deployment.

```bash
./g8e operator init                          # Build the operator binary inside g8e-pod (first time)
./g8e operator build                         # Rebuild the operator binary inside g8e-pod
./g8e operator deploy <user@host>            # Copy operator to remote host via scp (alias: drop)
./g8e operator stream <host...>              # Stream-inject operator to one or more remote hosts
./g8e operator ssh-config                    # Configure ~/.ssh/config for high-concurrency streaming
./g8e operator reauth --user-id <id>         # Kill and relaunch the g8e-pod operator for a user
```

| Subcommand | Delegates to | Notes |
|------------|-------------|-------|
| `init` | `docker exec g8e-pod go build ...` | Builds operator binary natively inside running g8e-pod |
| `build` | `docker exec g8e-pod go build ...` | Explicit rebuild of operator binary inside g8e-pod |
| `deploy` | `scp` + optional `ssh` | Alias: `drop`. Supports `--arch`, `--dest`, `--endpoint`, `--device-token`, `--key`, `--no-git` |
| `stream` | `docker exec g8e-pod /home/g8e/g8e.operator stream` | Zero local disk footprint. Supports `--arch`, `--hosts`, `--concurrency`, `--timeout`, `--endpoint`, `--device-token`, `--key`, `--no-git`, `--ssh-config` |
| `ssh-config` | `scripts/tools/setup-ssh.sh` | Configures multiplexing. Supports `--print`, `--force` |
| `reauth` | Internal API | Requires `--user-id <id>` or `--email <email>` |

#### test
Runs tests for platform components. Runs inside g8e-pod.

```bash
./g8e test                          # Run all component test suites
./g8e test vse                      # AI engine (Python/pytest)
./g8e test vsod                     # Dashboard (Node/Vitest)
./g8e test vsa                      # Operator (Go)
./g8e test security                 # Security scanning
./g8e test vse --coverage           # Generate coverage report
./g8e test vse --llm openai         # Run with a specific LLM provider
./g8e test vse --m <model>          # Override the LLM model for the run
./g8e test vsa -- TestFoo           # Pass extra args to the underlying test runner
```

#### security
Security validation and certificate management. Runs inside g8e-pod.

```bash
./g8e security certs                # Show TLS certificate status (default)
./g8e security certs generate       # Generate CA + server certificates
./g8e security certs rotate         # Rotate/regenerate existing certificates
./g8e security certs status         # Show certificate status and expiry
./g8e security certs trust          # Install CA into host OS store (host-side)
./g8e security mtls-test            # Run mTLS connectivity tests
./g8e security scan-licenses        # Scan all dependency licenses
./g8e security passkeys             # Manage user passkey credentials
./g8e security rotate-internal-token # Rotate the X-Internal-Auth shared secret across all components
```

#### data
Data management. All subcommands route through `manage-vsodb.py` inside g8e-pod.

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
./g8e data store stats                       # VSODB statistics
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
```

---

## Directory Structure

```
scripts/
  core/           # Platform build scripts (build.sh, setup.sh)
  data/           # Data management (unified via manage-vsodb.py dispatcher)
    _lib.py       #   Shared auth, HTTP clients, display helpers
    manage-vsodb.py  #   Entry point — routes to resource scripts
    manage-store.py  #   VSODB document store & KV queries
    manage-users.py  #   User CRUD
    manage-settings.py  # Platform settings
    manage-operators.py # Operator management
    manage-device-links.py  # Device link tokens
    manage-lfaa.py   #   LFAA audit vault (SQLite)
  security/       # TLS certificates and security validation
  testing/        # Test runner and supporting tools
  tools/          # Developer utilities (LLM setup)
```

---

## Core Build Script (`build.sh`)

**Location:** `scripts/core/build.sh`

Builds and runs the local VSO environment. Manages Docker Compose services in dependency order, waits for health checks, optionally wipes data volumes, handles parallel rebuilds, and stamps version into `VERSION` files on rebuild.

### Usage

```bash
./scripts/core/build.sh <command> [options]
```

### Commands

| Command | Description |
|---------|-------------|
| `status` | Show container status and component versions |
| `up [component...]` | Start managed services without building. Default: `vsodb vse vsod g8e-pod`. Auto-builds the operator binary inside g8e-pod if absent. |
| `down` | Stop managed containers (`vsodb`, `vse`, `vsod`, `g8e-pod`) |
| `rebuild [component...]` | No-cache rebuild + restart. Default: `vsodb vse vsod` |
| `wipe` | Remove data volumes for `vsodb`, `vse`, `vsod`, `g8e-pod` and restart. Auto-builds operator binary after restart. |
| `clean` | Remove all managed Docker resources (containers, images, volumes) |

---

## Testing (`run_tests.sh`)

**Location:** `scripts/testing/run_tests.sh`

Runs tests for VSO components inside the `g8e-pod` Docker container. Infrastructure must already be running.

### Usage

```bash
./scripts/testing/run_tests.sh [COMPONENT] [OPTIONS] [-- EXTRA_ARGS]
```

### Components

| Component | Test Framework | What It Tests |
|-----------|---------------|--------------|
| `vse` | pytest | VSE Python service |
| `vsod` | vitest / npm test | VSOD Node.js service |
| `vsa` | `gotestsum` | VSA Go binary |
| `security` | `security-validate.sh` | Operator binary security |
| `all` | All of the above | Full suite (default) |

### Options

| Option | Description |
|--------|-------------|
| `--coverage` | Generate coverage reports (HTML + JSON + terminal) |
| `--llm PROVIDER` | Override LLM provider: `openai`, `anthropic`, `gemini` |
| `--m MODEL` | Override LLM model name only |
| `-v` | Verbose output for VSA (`go test -v`) |

---

## Data Management

**Entry point:** `scripts/data/manage-vsodb.py`

All data operations route through a single dispatcher (`manage-vsodb.py`) that delegates to individual resource scripts. Each resource script can also run standalone. Shared HTTP clients, authentication, and display helpers live in `scripts/data/_lib.py`.

```
scripts/data/
  manage-vsodb.py        # Entry point — dispatches to resource scripts
  _lib.py                # Shared: auth, HTTP clients (VSODB + VSOD), display helpers
  manage-store.py        # VSODB document store & KV queries
  manage-users.py        # User CRUD via VSOD internal API
  manage-settings.py     # Platform settings read/write via VSOD internal API
  manage-operators.py    # Operator management via VSOD internal API
  manage-device-links.py # Device link token management via VSOD internal API
  manage-lfaa.py         # LFAA audit vault queries (local SQLite)
```

### Platform Settings (`settings`)

Read and display effective platform settings via the VSOD internal HTTP API. Secret values (API keys, tokens) are never returned. Shows the live effective value for each setting — env-locked entries are marked with `[env]`.

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

Manage operator documents via the VSOD internal HTTP API. All operations target the VSODB document store through VSOD — no direct DB access.

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

**`refresh-key`** terminates the old operator document (invalidating its API key and revoking its cert) and creates a new document at the same slot number with a fresh API key. This is the same operation VSOD performs when a user clicks "Refresh Key" in the UI.

**`reset`** deletes and recreates the operator document with default values, preserving the existing API key and slot number. Session state is cleared. Used for demo resets.

### Device Link Management (`device-links`)

Generate, list, and revoke device link tokens.

```bash
./g8e data device-links list --email user@example.com
./g8e data device-links create --email user@example.com --name "prod-fleet" --max-uses 50
./g8e data device-links revoke --token dlk_...
./g8e data device-links delete --token dlk_...
```

### VSODB Document Store (`store`)

Query the VSODB document store and KV store via the HTTP API. Runs inside g8e-pod and communicates with VSODB directly.

```bash
./g8e data store stats                                    # VSODB statistics
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

```bash
./g8e data audit --container operator-test-1 stats
./g8e data audit --container operator-test-1 sessions
./g8e data audit --container operator-test-1 events --session <id>
./g8e data audit --container operator-test-1 export --session <id> --out audit.json
```

---

## Security

### TLS Certificate Management (`manage-ssl.sh`)
**Location:** `scripts/security/manage-ssl.sh`

Manages the platform TLS certificates owned by VSODB. VSODB generates the CA and server certificates automatically on first start — this script orchestrates lifecycle via docker commands.

```bash
./g8e security certs generate   # Ensure certs exist (idempotent — no-op if already present)
./g8e security certs rotate     # Force-regenerate: wipe /data/ssl/ and restart VSODB
./g8e security certs status     # Show cert expiry, subject, and SANs
./g8e security certs trust      # Install CA into host OS trust store
```

After `rotate`, run `./g8e platform rebuild` to re-embed the new CA into the operator binary.

### Security Validation (`security-validate.sh`)
**Location:** `scripts/security/security-validate.sh`

### Internal Token Rotation

Rotate the internal auth token across all components. Uses `manage-vsodb.py settings rotate-token` internally.

```bash
./g8e security rotate-internal-token
```

### License Scanning (`scan-licenses.sh`)
**Location:** `scripts/security/scan-licenses.sh`

Scans dependencies for commercial distribution compatibility.

```bash
./g8e security scan-licenses
./g8e security scan-licenses --report        # Write CSV reports
```

### Windows CA Trust (`trust-ca.ps1`)
**Location:** `scripts/security/trust-ca.ps1`

One-shot PowerShell script for Windows users. Removes any previously installed g8e CA cert, fetches the new one from the platform via SSH, and installs it into `LocalMachine\Root` — all in one step.

Run this in an **Administrator PowerShell** prompt after each platform rebuild or cert rotation:

```powershell
.\trust-ca.ps1 -Server admin@10.0.0.2
```

The `-Server` parameter accepts a `user@host`, bare hostname, or SSH config alias.

---

### mTLS Verification (`mtls-test.sh`)
**Location:** `scripts/security/mtls-test.sh`

Verifies mTLS configuration for VSA ↔ VSODB communication.

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
```

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
