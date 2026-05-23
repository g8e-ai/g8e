---
title: Scripts
---

# g8e CLI

Last Updated: 2026-05-23

The unified `g8eo` binary is the primary operational interface for the g8e platform. It serves dual purposes: daemon mode (Governance Gateway/Operator) and CLI mode (platform management). The platform enforces a **host-native** execution model with **ZERO shell scripts** for platform operations.

---

## Architecture Overview

g8e avoids container-orchestration complexity by running directly on the host. There are two distinct tiers:

1. **Gateway (mandatory)** - The `g8eo` binary in Gateway mode (--doctrine, --consensus, or --notary). Owns persistence, PKI, pub/sub, and governance enforcement.
2. **Application Layer (optional)** - The reference **g8e Agentic Ensemble** (`g8ee`) that consume the public protocol. Run as managed host processes.

### The Unified `g8eo` Binary

The `g8eo` binary is a statically compiled Go binary that serves dual purposes:

- **Daemon mode**: Runs the Governance Gateway/Operator when invoked without subcommands
- **CLI mode**: Manages platform lifecycle, auth, data, and tests when invoked with subcommands

- **Host runtime state** - All runtime data lives at `./.g8e/`: `data/`, `pki/`, `secrets/`, `logs/`.
- **Credentials** - Authenticated commands use `~/.g8e/credentials`.

Running `./g8e` (a symlink to the g8eo binary) without arguments launches the Interactive Platform Manager. Direct command form: `./g8e <command> [subcommand] [options]`.

---

## Command Categories

### Platform Management - `./g8e platform`

Orchestrates the Gateway lifecycle via native Go process management.

| Command | Purpose |
|---|---|
| `start [-a\|--g8ee\|--with-g8ee]` | Start Operator Gateway mode; optional apps require explicit opt-in. |
| `stop` | Stop Operator Gateway mode and any optional app processes. |
| `restart` | Restart with the same flags. |
| `status` | Gateway health first, optional app status separately. |
| `wipe` | Clears app data via the Operator API. Preserves PKI, secrets, settings, and auth state. |
| `reset` | Destructive: wipes data and bootstrap secrets. **Preserves PKI roots.** |
| `clean` | Nuke all processes and the entire `.g8e/` runtime directory. |
| `logs` | Stream aggregated logs from `./.g8e/logs/`. |
| `settings` | Read or update platform configuration. |

### Application Layer - `./g8e apps`

Manages optional, opt-in adapters.

| Command | Purpose |
|---|---|
| `start [g8ee\|all]` | Start an optional app. |
| `stop [g8ee\|all]` | Stop an optional app. |
| `restart [g8ee\|all]` | Restart an optional app. |
| `status` | App status alongside Gateway status. |
| `build [g8ee\|all]` | Install native deps (e.g., Python venv). |

Apps are BYO clients with no Gateway responsibilities and no private coupling.

### Operator Operations - `./g8e operator`

Lifecycle for `g8eo` binaries and remote fleet deployment.

| Command | Purpose |
|---|---|
| `init` | Compile the operator for the local architecture. |
| `build` / `build-all` | Cross-compile for amd64/arm64/386. UPX-compresses and syncs to the Hub blob store. |
| `deploy <user@host>` | Fetch the signed binary from the local hub and SCP/SSH it to a remote host. |
| `stream <host...>` | High-concurrency fleet-wide injection over SSH. |
| `reauth` | Trigger re-authentication of a running operator process. |
| `ssh-config` | Manage SSH identities for fleet operations. |

### Identity - `./g8e login` / `./g8e logout`

`login` mints CLI cert + key, captures session id, and writes credentials to `~/.g8e/credentials`. `logout` clears local session and credentials.

### Chat - `./g8e chat [prompt]`

Starts an interactive web session with the **g8e Agentic Ensemble**. Optional initial prompt.

### Variables - `./g8e vars`

| Command | Purpose |
|---|---|
| `list` / `ls` | List all g8e env vars and current values. |
| `set <key> <value>` | Set a variable in `.g8e/.env`. |
| `get <key>` | Display a variable. |
| `unset <key>` | Remove a variable. |

### Data & Security - `./g8e data` / `./g8e security`

**`data`** - Native Go client for Gateway state over mTLS:

- `users` - User and session management.
- `operators` - Operator registration and slot management.
- `store <collection> list|get` - Document store and KV queries.
- `device-links` - Device-link token lifecycle.
- `audit` - LFAA git-ledger and audit vault queries.
- `settings` - Low-level platform configuration.

**`security`** - TLS and identity invariants:

- `validate` - PKI integrity and environment consistency.
- `mtls-test` - Connectivity test for mTLS trust.
- `passkeys` - WebAuthn/FIDO2 hardware-bound identity management.
- `scan-licenses` - Dependency license compliance.

### Testing - `./g8e test`

See [Tests](./tests.md). Native toolchains via the unified Go CLI.

| Command | Purpose |
|---|---|
| `g8eo [path]` | Go Operator Gateway tests with race detection. **Default when no component is provided.** |
| `g8ee [path]` | Optional **g8e-compliant agentic ensemble** tests with LLM provider support. |
| `chaos [options]` | Resiliency testing via `chaos_tester` (e.g., `--count=100`). |

### Evals - `./g8e evals`

See [Evals](./evals.md).

| Command | Purpose |
|---|---|
| `bench --suite <suite> --mode <baseline\|receipt>` | Run a benchmark suite. |
| `verify-receipts <report-dir>` | Re-verify receipt signatures offline. |
| `list` | List benchmark suites and bundled gold sets. |

### Demo - `./g8e demo`

See [Demos](./demos.md).

| Command | Purpose |
|---|---|
| `deploy [-n <count>] -d <token>` | Start and authenticate a simulated fleet of N devices. |
| `down` | Stop all simulation nodes. |
| `status` | Container status and node counts. |
| `clean` | Forcefully remove all demo artifacts. |
| `profile [list\|switch]` | Manage demo scenarios. |
| `shell <node>` | Drop into a simulation node's shell. |
| `devices` / `broken` | List discovered or unhealthy devices. |
| `operators` | Status of g8e operator processes in the fleet. |

### LLM - `./g8e llm`

| Command | Purpose |
|---|---|
| `setup` | Interactive provider configuration. |
| `show` / `get` / `set` | View or update LLM variables. |
| `restart` | Restart Ensemble to apply settings. |

### Integrations - `./g8e mcp` / `./g8e search` / `./g8e ssh` / `./g8e aws`

- `mcp` - Model Context Protocol integration (`config`, `test`, `status`).
- `search` - Vertex AI Search configuration (`setup`, `disable`).
- `ssh` - Manage host SSH key mounts.
- `aws` - Manage AWS credential mounts.

---

## Zero Shell Scripts Policy

**CRITICAL**: The platform uses ZERO shell scripts for platform operations. All platform lifecycle, configuration, and administrative duties are handled by the unified Go binary (`./g8e`).

**Permissible Shell Scripts:**
- Service entrypoints (`services/*/entrypoint.sh`) - minimal wrappers that set environment and exec the binary
- Build-time toolchain scripts (`scripts/ingest/*.py`, `scripts/docs/*.sh`) - disconnected from runtime platform operations
- Vendor scripts (third-party Go vendor scripts) - not g8e platform code

**Deleted Scripts (2026-05-23):**
- All `scripts/core/*` shell scripts (config.sh, path_utils.sh, json_query.py, logs.sh, manage-env.sh, setup.sh, local_setup.sh, stream_events.py)
- All `scripts/cmd/*` shell scripts (common.sh, env_vars.sh, paths.sh, api_paths.sh, headers.sh)
- All `scripts/tools/*` shell scripts (approve-transaction.sh, setup-llm.sh, setup-search.sh, setup-ssh.sh)
- All `scripts/docs/*` shell scripts (generate_cli_reference.sh)
- All `scripts/security/*` shell scripts (validate-platform-security.sh)
- All `scripts/testing/*` shell scripts (run_tests.sh, test_test_help.sh)
- `evals/tests/byo_client_parity.sh`
- `demo/profiles/*`

---

## Technical Invariants

1. **Zero Shell Scripts**: NO shell scripts are used for platform operations. All platform lifecycle, configuration, and administrative duties are handled by the unified Go binary.
2. **Service readiness** - The platform is not "ready" until the Gateway `/healthz` passes. The Go CLI blocks until this state.
3. **Canonical wire format** - All client-facing interaction uses canonical JSON (protojson). Binary protobuf is reserved for internal storage.
4. **Fail-closed execution** - The CLI must never mask failures or proceed with missing trust material. Missing trust bundles or secrets exit with an actionable error pointing at the platform Gateway.

For detailed help on any subcommand: `./g8e <command> --help`.

See also: [Operator](../concepts/operator.md), [Governance Gateway (g8eg)](../concepts/g8eg.md), [Tests](./tests.md), [Evals](./evals.md).
