---
title: Scripts
---

# g8e Scripts

Last Updated: 2026-05-18

The scripts layer is the primary operational interface for the g8e platform. It enforces a **host-native** execution model and manages the lifecycle of the mandatory Operator substrate plus optional application-layer adapters.

---

## Architecture Overview

g8e avoids container-orchestration complexity by running directly on the host. There are two distinct tiers:

1. **Substrate (mandatory)** - The `g8eo` binary in `--listen` mode. Owns persistence, PKI, pub/sub, and governance enforcement.
2. **Application Layer (optional)** - Reference adapters like `g8ee` that consume the public protocol. Run as managed host processes.

### The `./g8e` CLI entry point

The root `./g8e` script is a Bash-based dispatcher and the single entry point for all platform operations.

- **Global flags** - `--api-key` / `-k` (explicit API key), `--device-token` (device-link token), `--dev` (skip prod-readiness gates).
- **Host runtime state** - All runtime data lives at `./.g8e/`: `data/`, `pki/`, `secrets/`, `logs/`.
- **Credentials** - Authenticated commands use `~/.g8e/credentials`.

Running `./g8e` without arguments launches the Interactive Platform Manager. Direct command form: `./g8e <command> [subcommand] [options]`.

---

## Command Categories

### Platform Management - `./g8e platform`

Orchestrates the substrate lifecycle via `scripts/core/build.sh`.

| Command | Purpose |
|---|---|
| `start [-a\|--with-apps\|--with-g8ee]` | Start Operator listen mode; optional apps require explicit opt-in. |
| `stop` | Stop Operator listen mode and any optional app processes. |
| `restart` | Restart with the same flags. |
| `status` | Substrate health first, optional app status separately. |
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
| `status` | App status alongside substrate status. |
| `build [g8ee\|all]` | Install native deps (e.g., Python venv). |

Apps are BYO clients with no substrate responsibilities and no private coupling.

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

Starts an interactive web session with the AI Engine. Optional initial prompt.

### Variables - `./g8e vars`

| Command | Purpose |
|---|---|
| `list` / `ls` | List all g8e env vars and current values. |
| `set <key> <value>` | Set a variable in `.g8e/.env`. |
| `get <key>` | Display a variable. |
| `unset <key>` | Remove a variable. |

### Data & Security - `./g8e data` / `./g8e security`

Dispatched via `scripts/cmd/infra.sh`.

**`data`** - Python helpers for substrate state:

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

See [Tests](tests.md). Native toolchains via `scripts/testing/run_tests.sh`.

| Command | Purpose |
|---|---|
| `g8eo [path]` | Go Operator substrate tests with race detection. **Default when no component is provided.** |
| `g8ee [path]` | Optional Python Engine adapter tests with LLM provider support. |
| `chaos [options]` | Resiliency testing via `chaos_tester` (e.g., `--count=100`). |

### Evals - `./g8e evals`

See [Evals](evals.md).

| Command | Purpose |
|---|---|
| `bench --suite <suite> --mode <baseline\|receipt>` | Run a benchmark suite. |
| `verify-receipts <report-dir>` | Re-verify receipt signatures offline. |
| `list` | List benchmark suites and bundled gold sets. |

### Demo - `./g8e demo`

See [Demos](demos.md).

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
| `restart` | Restart inference engine to apply settings. |

### Integrations - `./g8e mcp` / `./g8e search` / `./g8e ssh` / `./g8e aws`

- `mcp` - Model Context Protocol integration (`config`, `test`, `status`).
- `search` - Vertex AI Search configuration (`setup`, `disable`).
- `ssh` - Manage host SSH key mounts.
- `aws` - Manage AWS credential mounts.

---

## Directory Structure

```text
scripts/
├── cmd/           # Primary command implementations (Bash)
│   ├── platform.sh   # Substrate lifecycle
│   ├── apps.sh       # App-layer lifecycle
│   ├── infra.sh      # Data/Security dispatcher
│   ├── operator.sh   # Operator binary/deployment
│   └── tests.sh      # Test execution bridge
├── core/          # Internal orchestrators
│   ├── build.sh      # Main process manager
│   ├── manage-env.sh # Variable resolution
│   └── path_utils.sh # PROJECT_ROOT resolution
├── data/          # Substrate interaction (Python)
├── security/      # Security logic
├── testing/       # Test runners
└── tools/         # Setup wizards (LLM, SSH, Search)
```

---

## Technical Invariants

1. **Path resolution** - Scripts resolve `G8E_PROJECT_ROOT` relative to their own location.
   - Bash: `$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)`
   - Python: `Path(__file__).parent.parent.parent.absolute()`
2. **Service readiness** - The platform is not "ready" until the Hub `/healthz` passes. `build.sh` blocks until this state.
3. **Canonical wire format** - All client-facing interaction uses canonical JSON (protojson). Binary protobuf is reserved for internal storage.
4. **Fail-closed execution** - Scripts never mask failures or proceed with missing trust material. Missing trust bundles or secrets exit with an actionable error pointing at the platform substrate.

For detailed help on any subcommand: `./g8e <command> --help`.

See also: [Operator](operator.md), [g8eo Service](g8eo_service.md), [Tests](tests.md), [Evals](evals.md), [Demos](demos.md).
