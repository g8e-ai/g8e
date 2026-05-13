---
title: Scripts
parent: Architecture
---

# g8e Scripts

Last Updated: 2026-05-11
Version: v0.2.4

The scripts layer provides the operational interface for the g8e platform. It has been migrated from container orchestration to host-native lifecycle management for Operator, Dashboard (g8ed), and Engine (g8ee).

## Architecture Overview

g8e uses a host-native target model. Operator listen mode owns local persistence and pub/sub state under `./.g8e`, while Dashboard and Engine run as managed platform services on the host.

### The `./g8e` CLI Entry Point

The root `./g8e` script is a Bash-based dispatcher. It is the only script an operator should invoke directly on the host.

- **Host Runtime State:** Generated platform runtime state is rooted at `./.g8e`, including `data`, `pki`, `secrets`, `pids`, and `logs`. Tooling receives trust material through `G8E_TRUST_BUNDLE` and bootstrap secrets through `G8E_SECRETS_DIR`.
- **Session Management:** Commands targeting the internal API (`data`, `security`, `mcp`, `operator`) are gated by a local credential store (`~/.g8e/credentials`) which is populated via `./g8e login`.

### Execution Flow
1. **Host-Side:** Commands like `platform start` or `operator build` run directly on the host, managing Operator listen mode and component lifecycle.
2. **Tooling-Side:** Commands like `data users list` or `security validate` run with the platform environment populated, including internal URLs, the host trust bundle, and the host secrets directory, often delegating to Python or Node.js helpers.

---

## Core Command Categories

### Platform Management (`./g8e platform`)
Orchestrates platform lifecycle via `scripts/core/build.sh`.

- **`start` / `stop` / `restart`:** Host-native service lifecycle. `start` waits for service health checks.
- **`rebuild`:** Restarts managed services (g8ee, g8ed). Unlike legacy container models, this does not build images but restarts host processes.
- **`setup`:** Initial platform configuration and service initialization.
- **`reset`:** Destructive. Wipes Dashboard/Engine data, Operator listen-mode data, and bootstrap secrets while preserving PKI material in `./.g8e/pki`.
- **`wipe`:** Clears application data via the Operator listen-mode API. Preserves platform settings, PKI material, secrets, and authentication state.
- **`logs`:** Aggregates logs from all managed services into a single stream via `scripts/core/logs.sh`.
- **`settings`:** Direct access to platform-wide settings stored by Operator listen mode.

### Data Management (`./g8e data`)
Unified interface for interacting with platform state, dispatched via `scripts/data/manage-operator.py`.

- **Dispatcher Pattern:** `manage-operator.py` routes requests to specialized modules:
    - **`store`**: Document store & KV queries (`manage-store.py`).
    - **`users`**: Platform user management (`manage-users.py`).
    - **`operators`**: Operator document management (`manage-operators.py`). Note: Operator API keys are write-only operational secrets; they are never displayed or retrieved by this script.
    - **`device-links`**: Device link token management (`manage-device-links.py`).
    - **`settings`**: Platform settings read/write (`manage-settings.py`).
    - **`audit`**: LFAA audit vault queries (`manage-lfaa.py`).
    - **`mcp`**: MCP client integration configuration (`manage-mcp.py`).
    - **`reputation`**: Reputation state & commitment management (`manage-reputation.py`).

### Security & Identity (`./g8e security`)
Manages the platform's root of trust and security invariants.

- **`validate`:** Checks TLS integrity, permissions, and environment consistency.
- **`pki`:** Operator-owned PKI management and trust bundle operations.
- **`mtls-test`:** Connectivity test for mTLS between components.
- **`passkeys`:** Manages FIDO2/WebAuthn credentials via `manage-passkeys.py`.

### Testing (`./g8e test`)
Runs component-specific test suites using native toolchains via `scripts/testing/run_tests.sh`.

- **Isolation:** Tests for `g8ee` (Python), `g8ed` (Node.js), and `g8eo` (Go) run on the host using native virtualenvs, npm, and Go toolchains.

### Operator Operations (`./g8e operator`)
Lifecycle management for the `g8eo` operator binary and remote deployments.

- **`build` / `build-all`:** Compiles the operator for current or all architectures (amd64, arm64, 386).
- **`deploy`**: Fetches the binary from Operator listen mode and copies it to a remote host via SSH.
- **`stream`**: Starts an interactive streaming session with remote operators.
- **`reauth`**: Forces a re-authentication of a running operator process.

### Interactive Setup Tools
Located in `scripts/tools/`, these provide guided configuration:

- **`./g8e llm setup`**: Configures LLM providers (Gemini, Anthropic, OpenAI, etc.).
- **`./g8e search setup`**: Configures Vertex AI Search for web grounding.
- **`./g8e ssh setup`**: Configures SSH multiplexing for operator streaming.

---

## Directory Structure

```text
scripts/
├── core/           # Platform lifecycle (Bash)
│   ├── build.sh    #   Platform lifecycle orchestration
│   ├── logs.sh     #   Log aggregation
│   └── setup.sh    #   Setup delegation
├── data/           # Data operations (Python)
│   ├── manage-operator.py    # Main dispatcher
│   ├── manage-store.py       # Document/KV queries
│   ├── manage-users.py       # User management
│   ├── manage-lfaa.py        # Audit vault queries
│   └── manage-reputation.py  # Reputation management
├── security/       # TLS and Security (Bash/Python)
│   ├── manage-pki.sh         # Operator PKI validation and lifecycle helpers
│   ├── manage-passkeys.py    # FIDO2/WebAuthn
│   └── validate-platform-security.sh
├── testing/        # Test runners
│   └── run_tests.sh          # Main test execution bridge
└── tools/          # Setup wizards
    ├── setup-llm.sh          # LLM provider config
    ├── setup-search.sh       # Vertex Search config
    └── setup-ssh.sh          # SSH multiplexing config
```

---

## Technical Invariants

### 2. Path Resolution
All scripts must resolve `PROJECT_ROOT` relative to their own location to ensure they work regardless of the user's current working directory.
- **Bash:** `$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)`
- **Python:** `Path(__file__).parent.parent.parent.absolute()`

### 3. Service Readiness
The platform does not consider itself "up" until Operator listen mode and component readiness checks pass. `build.sh` waits for these checks before completing `up` or `start` commands.
