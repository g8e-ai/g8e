---
title: Scripts
parent: Architecture
---

# g8e Scripts

Last Updated: 2026-05-13
Version: v0.2.5

The scripts layer provides the primary operational interface for the g8e platform. It enforces a **host-native** execution model, managing the lifecycle of the mandatory Operator substrate and optional application-layer adapters.

## Architecture Overview

g8e avoids container orchestration complexity by running directly on the host. The architecture is split into two distinct tiers:

1. **Substrate (Mandatory):** The `g8eo` Operator binary running in `--listen` mode. It owns persistence, PKI, messaging (PubSub), and governance enforcement.
2. **Application Layer (Optional):** Reference adapters like `g8ee` (AI Engine) that consume the protocol. These run as managed host processes.

### The `./g8e` CLI Entry Point

The root `./g8e` script is a Bash-based dispatcher. It is the single entry point for all platform operations.

- **Global Flags:**
    - `--api-key` / `-k`: Explicit API key for authentication.
    - `--device-token`: Device-link token for initial registration.
    - `--dev`: Enable development mode (e.g., skip production readiness gates).
- **Host Runtime State:** All runtime data is rooted at `./.g8e`, including:
    - `data/`: SQLite databases and KV stores.
    - `pki/`: Platform Certificate Authority and trust bundles.
    - `secrets/`: Bootstrap secrets and identity materials.
    - `logs/`: Aggregated service logs.
- **Credential Management:** Authenticated commands target the Operator's internal API, using credentials stored in `~/.g8e/credentials`.

---

## Core Command Categories

### Platform Management (`./g8e platform`)
Orchestrates the mandatory substrate lifecycle via `scripts/core/build.sh`.

- **`start` / `stop` / `restart`:** Manages the Operator `--listen` process and its dependencies.
- **`status`:** Health check for the substrate and any active apps.
- **`reset`:** Destructive. Wipes application data and bootstrap secrets while preserving PKI roots.
- **`wipe`:** Clears application-layer data via the Operator API.
- **`clean`:** Complete removal of all g8e processes and data.
- **`logs`:** Streams aggregated logs from `./.g8e/logs/`.
- **`settings`:** Read-only access to platform-wide configuration.

### Application Layer (`./g8e apps`)
Manages optional, opt-in adapters that extend the platform's capabilities.

- **`start` / `stop` / `restart` [g8ee|all]:** Component-specific lifecycle.
- **`build`:** Installs native dependencies (e.g., Python virtualenvs for `g8ee`).
- **Note:** Apps are "BYO clients" — they have no substrate responsibilities and no private coupling.

### Operator Operations (`./g8e operator`)
Lifecycle management for `g8eo` binaries and remote fleet deployment.

- **`init`:** Compiles the operator for the local architecture.
- **`build` / `build-all`:** Cross-compiles for amd64, arm64, and 386.
- **`deploy <user@host>`:** Fetches the signed binary from the local hub and copies it to a remote host via SSH.
- **`stream`:** Interactive session for managing remote operators via the Hub's PubSub broker.
- **`reauth`:** Triggers a re-authentication of a running operator process.

### Infrastructure & Data (`./g8e data` / `./g8e security`)
Unified interface for interacting with the substrate, dispatched via `scripts/cmd/infra.sh`.

- **`data`**: Specialized Python helpers for substrate state:
    - `users`: User and session management.
    - `operators`: Operator registration and slot management.
    - `store`: Document store and KV queries.
    - `device-links`: Device-link token lifecycle.
    - `audit`: LFAA git-ledger and audit vault queries.
- **`security`**: TLS and identity invariants:
    - `validate`: Checks PKI integrity and environment consistency.
    - `mtls-test`: Connectivity test for mTLS trust.
    - `passkeys`: WebAuthn/FIDO2 hardware-bound identity management.

### Testing (`./g8e test`)
Runs component tests using native toolchains via `scripts/testing/run_tests.sh`.

- **`g8eo`**: Go unit and integration tests.
- **`g8ee`**: Python tests (pytest), including optional E2E and linting (Ruff/Pyright).
- **`chaos`**: Resiliency testing via `chaos_tester`.

---

## Directory Structure

```text
scripts/
├── cmd/           # Primary command implementations (Bash)
│   ├── platform.sh # Substrate lifecycle
│   ├── apps.sh     # App-layer lifecycle
│   ├── infra.sh    # Data/Security dispatcher
│   ├── operator.sh # Operator binary/deployment
│   └── tests.sh    # Test execution bridge
├── core/          # Internal orchestrators
│   ├── build.sh    # Main process manager
│   ├── manage-env.sh # Variable resolution
│   └── path_utils.sh # PROJECT_ROOT resolution
├── data/          # Substrate interaction (Python)
│   ├── manage-users.py
│   ├── manage-operators.py
│   └── manage-lfaa.py
├── security/      # Security logic
│   └── validate-platform-security.sh
├── testing/       # Test runners
│   └── run_tests.sh
└── tools/         # Setup wizards (LLM, SSH, Search)
```

---

## Technical Invariants

### 1. Path Resolution
All scripts must resolve `G8E_PROJECT_ROOT` relative to their own location.
- **Bash:** `$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)`
- **Python:** `Path(__file__).parent.parent.parent.absolute()`

### 2. Service Readiness
The platform is not "ready" until the Operator listen-mode health check (`/healthz`) passes. `build.sh` blocks until this state is reached.

### 3. Canonical Wire Format
All client-facing interaction (HTTP, PubSub, receipts) must use **canonical JSON (protojson)**. Binary Protobuf is reserved for internal storage and audit vaults to ensure the platform remains accessible to generic BYO clients (MCP, A2A, LLMs).

### 4. Fail-Closed Execution
Scripts must never mask failures or proceed with missing trust material. If a trust bundle or secret is missing, the script must exit with a clear error instructing the user to restart the platform substrate.
