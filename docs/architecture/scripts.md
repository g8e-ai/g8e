---
title: Scripts
parent: Architecture
---

# g8e Scripts

Last Updated: 2026-05-07
Version: v0.2.0

The scripts layer provides the operational interface for the g8e platform. It abstracts the complexity of a multi-container environment into a unified CLI, ensuring consistent execution across development, testing, and production environments.

## Architecture Overview

g8e uses a **host-bridged execution model**. While the platform runs in Docker, most operational tasks are executed inside dedicated ephemeral containers or via direct API calls to ensure environment consistency.

### The `./g8e` CLI Entry Point

The root `./g8e` script is a Bash-based dispatcher. It is the only script an operator should invoke directly on the host.

- **Dependency-Free Host:** The host requires only `docker` and `docker-compose`. No Python, Go, or Node.js toolchains are needed locally.
- **Session Management:** Commands targeting the internal API (`data`, `security`, `mcp`, `operator`) are gated by a local credential store (`~/.g8e/credentials`) which is populated via `./g8e login`.

### Execution Flow
1. **Host-Side:** Commands like `platform start` or `operator build` run directly on the host, orchestrating Docker Compose or triggering builds in specialized test-runners.
2. **Container-Side:** Commands like `data users list` or `security validate` are forwarded via `docker run` or `docker exec` into ephemeral runner containers, where they have access to:
   - Internal service networks (`g8es`, `g8ed`).
   - Mounted secrets (TLS certs, internal auth tokens).
   - The full Python operational toolchain.

---

## Core Command Categories

### Platform Management (`./g8e platform`)
Orchestrates the lifecycle of the Docker Compose environment via `scripts/core/build.sh`.

- **`start` / `stop` / `restart`:** Basic container lifecycle. `start` waits for service health checks.
- **`build` / `rebuild`:** Creates or recreates images. `rebuild` automatically syncs agent personas before rebuilding the `g8ee` image.
- **`setup`:** Initial platform configuration and volume initialization.
- **`reset`:** Destructive. Wipes all data volumes (g8es, g8ee, g8ed) and rebuilds from scratch. **TLS certificates in `g8es-ssl` are preserved.**
- **`wipe`:** Clears application data via the `g8es` API. Preserves platform settings, SSL certs, and authentication state. Useful for demo resets.
- **`logs`:** Aggregates logs from all managed services into a single, time-ordered stream via `scripts/core/logs.sh`.
- **`settings`:** Direct access to platform-wide settings (LLM, search, etc.) stored in `g8es`.

### Data Management (`./g8e data`)
Unified interface for interacting with platform state, dispatched via `scripts/data/manage-g8es.py`.

- **Dispatcher Pattern:** `manage-g8es.py` routes requests to specialized modules:
    - **`store`**: Document store & KV queries (`manage-store.py`).
    - **`users`**: Platform user management (`manage-users.py`).
    - **`operators`**: Operator document management (`manage-operators.py`).
    - **`device-links`**: Device link token management (`manage-device-links.py`).
    - **`settings`**: Platform settings read/write (`manage-settings.py`).
    - **`audit`**: LFAA audit vault queries (`manage-lfaa.py`).
    - **`mcp`**: MCP client integration configuration (`manage-mcp.py`).
    - **`reputation`**: Reputation state & commitment management (`manage-reputation.py`).
- **`sync-personas`:** Unidirectional sync from Python persona models (`components/g8ee/app/models/personas/`) to the canonical `agents.json`.

### Security & Identity (`./g8e security`)
Manages the platform's root of trust and security invariants.

- **`validate`:** Checks TLS integrity, volume mount permissions, and environment variable consistency.
- **`certs`:** Manages the internal ECDSA P-384 CA via `scripts/security/manage-ssl.sh`.
- **`mtls-test`:** Connectivity test for mTLS between components.
- **`passkeys`:** Manages FIDO2/WebAuthn credentials via `manage-passkeys.py`.
- **`rotate-internal-token`:** Rotates the `X-Internal-Auth` token used for service-to-service communication.

### Testing (`./g8e test`)
Runs component-specific test suites in isolated runner containers via `scripts/testing/run_tests.sh`.

- **Isolation:** Tests for `g8ee` (Python), `g8ed` (Node.js), and `g8eo` (Go) run in dedicated containers.

### Operator Operations (`./g8e operator`)
Lifecycle management for the `g8eo` operator binary.

- **`build` / `build-all`:** Compiles the operator for current or all architectures (amd64, arm64).
- **`deploy`**: Fetches the binary from `g8es` and copies it to a remote host via SSH.
- **`stream`**: Starts an interactive streaming session with one or more remote operators.
- **`reauth`**: Forces a re-authentication of a running operator process.

### Interactive Setup Tools
Located in `scripts/tools/`, these provide guided configuration:

- **`./g8e llm setup`**: Configures LLM providers (Gemini, Anthropic, OpenAI, etc.).
- **`./g8e search setup`**: Configures Vertex AI Search for web grounding.
- **`./g8e aws setup`**: Mounts local AWS credentials into the platform.
- **`./g8e ssh setup`**: Configures SSH multiplexing for operator streaming.

---

## Directory Structure

```text
scripts/
├── core/           # Platform lifecycle (Bash)
│   ├── build.sh    #   Docker Compose orchestration
│   ├── logs.sh     #   Log aggregation
│   └── setup.sh    #   Environment initialization
├── data/           # Data operations (Python)
│   ├── manage-g8es.py    # Main dispatcher
│   ├── manage-store.py   # Document/KV queries
│   ├── manage-users.py   # User management
│   ├── manage-lfaa.py    # Audit vault queries
│   └── sync-personas.py  # Persona synchronization
├── security/       # TLS and Security (Bash/Python)
│   ├── manage-ssl.sh     # Cert lifecycle
│   ├── manage-passkeys.py # FIDO2/WebAuthn
│   ├── mtls-test.sh      # mTLS validation
│   └── validate-platform-security.sh
├── testing/        # Test runners and parity
│   └── run_tests.sh      # Main test execution bridge
└── tools/          # Setup wizards
    ├── setup-llm.sh      # LLM provider config
    ├── setup-search.sh   # Vertex Search config
    └── setup-ssh.sh      # SSH multiplexing config
```

---

## Technical Invariants

### 1. Internal Authentication
All scripts running inside `g8ep` authenticate to `g8es` and `g8ed` using a shared `X-Internal-Auth` token. This token is mounted at `/g8es/internal_auth_token` and is rotated via `./g8e security rotate-internal-token`.

### 2. Path Resolution
All scripts must resolve `PROJECT_ROOT` relative to their own location to ensure they work regardless of the user's current working directory.
- **Bash:** `$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)`
- **Python:** `Path(__file__).parent.parent.parent.absolute()`

### 3. Service Readiness
The platform does not consider itself "up" until Docker health checks return `healthy`. `build.sh` waits for these checks before completing `up` or `start` commands.
