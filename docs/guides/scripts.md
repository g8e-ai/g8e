---
title: Scripts
---

# g8e CLI

Last Updated: 2026-05-25

The g8e CLI is the primary operational interface for the Governance Gateway (g8eg) and Governed Operator (g8eo). It is a statically compiled Go binary that manages platform lifecycle, authentication, data queries, and testing. The platform enforces a **host-native** execution model with **ZERO shell scripts** for platform operations.

---

## Architecture Overview

g8e runs directly on the host without container-orchestration complexity. The CLI serves dual purposes:

- **Daemon mode**: Runs the Governance Gateway when invoked without subcommands
- **CLI mode**: Manages platform lifecycle, auth, data, and tests when invoked with subcommands

**Host runtime state** - All runtime data lives at `./.g8e/`: `data/`, `pki/`, `secrets/`, `logs/`.
**Credentials** - Authenticated commands use `~/.g8e/credentials`.

Command form: `./g8e <command> [subcommand] [options]`.

---

## Commands

### setup

Bootstrap platform dependencies and build services.

```bash
./g8e setup
```

Checks for required dependencies (Go, Python), generates protocol artifacts, and builds services.

---

### platform

Manage the Governance Gateway (g8eg) daemon lifecycle.

| Subcommand | Purpose |
|---|---|
| `start` | Start the Governance Gateway |
| `stop` | Stop the Governance Gateway |
| `status` | Check Gateway health and status |
| `restart` | Restart the Governance Gateway |
| `logs` | View Gateway logs |
| `settings` | Manage Gateway settings |
| `reset` | Reset Gateway data and secrets (preserves CA) |
| `clean` | Destructively remove all Gateway state |

**Examples:**

```bash
./g8e platform start
./g8e platform status
./g8e platform logs
./g8e platform reset --force
./g8e platform clean --force
```

---

### auth

Authentication, session management, and PKI enrollment.

| Subcommand | Purpose |
|---|---|
| `bootstrap` | Bootstrap the platform with initial user and certificates |
| `login` | Authenticate and save operator session |
| `logout` | Clear local operator session and credentials |

**Examples:**

```bash
./g8e auth bootstrap
./g8e auth login --count=1 --ttl=3600
./g8e auth logout
```

The `bootstrap` command creates the first user and issues mTLS certificates for the Operator and CLI. It is only available over loopback when no users exist. The `login` command authenticates via device-link token and saves mTLS credentials to `~/.g8e/credentials`.

---

### data

Query local database collections, users, operators, and device-links over mTLS.

| Subcommand | Purpose |
|---|---|
| `users` | Manage user accounts |
| `operators` | Manage operator instances |
| `device-links` | Manage device-link tokens (list, create, delete) |
| `settings` | Manage Gateway settings |
| `store` | Manage document storage |
| `audit` | Query audit vault (list, summary) |

**Examples:**

```bash
./g8e data users
./g8e data operators
./g8e data device-links list --user-id=<id>
./g8e data device-links create --user-id=<id> --count=1 --ttl=3600
./g8e data device-links delete --token=<token> --user-id=<id>
./g8e data settings
./g8e data store --collection=<name> --document-id=<id>
./g8e data audit list --operator-session-id=<id> --limit=100
./g8e data audit summary
```

---

### test

Orchestrate unit, scenario, and chaos tests.

| Subcommand | Purpose |
|---|---|
| `unit` | Run unit tests |
| `integration` | Run integration tests |
| `g8eo` | Run Gateway (g8eo) tests |
| `ci` | Run CI test suite (g8eo) |
| `chaos` | Run chaos engineering tests |
| `scenario` | Run scenario integration tests |

**Examples:**

```bash
./g8e test unit --race --v
./g8e test integration --run=<scenario>
./g8e test g8eo --race --v --run=<test>
./g8e test ci
./g8e test chaos --count=100
./g8e test scenario --run=<scenario>
```

The default `./g8e test` command runs all unit and integration tests.

---

### security

Certificate/PKI validation, mTLS connectivity testing, and WebAuthn/Passkey registration.

| Subcommand | Purpose |
|---|---|
| `validate` | Run security validation checks |

**Examples:**

```bash
./g8e security validate --pki-dir=.g8e/pki --secrets-dir=.g8e/secrets
```

The `validate` command checks PKI directory structure, secrets directory, certificate validity, port availability, and TLS configuration.

---

### vars

Environment variable configuration.

| Subcommand | Purpose |
|---|---|
| `list` / `ls` | List all g8e environment variables |
| `set <key> <value>` | Set a variable in `.g8e/.env` |
| `get <key>` | Display the value of a specific variable |
| `unset <key>` | Remove a variable from `.g8e/.env` |

**Examples:**

```bash
./g8e vars list
./g8e vars set G8E_LOG_LEVEL debug
./g8e vars get G8E_LOG_LEVEL
./g8e vars unset G8E_LOG_LEVEL
```

---

## Zero Shell Scripts Policy

**CRITICAL**: The platform uses ZERO shell scripts for platform operations. All platform lifecycle, configuration, and administrative duties are handled by the unified Go binary (`./g8e`).

**Permissible Shell Scripts:**
- Vendor scripts (third-party Go vendor scripts in `vendor/` and `tools/vendor/`) - not g8e platform code

---

## Technical Invariants

1. **Zero Shell Scripts**: NO shell scripts are used for platform operations. All platform lifecycle, configuration, and administrative duties are handled by the unified Go binary.
2. **Service readiness** - The platform is not "ready" until the Gateway `/healthz` passes. The Go CLI blocks until this state.
3. **Canonical wire format** - All client-facing interaction uses canonical JSON (protojson). Binary protobuf is reserved for internal storage.
4. **Fail-closed execution** - The CLI must never mask failures or proceed with missing trust material. Missing trust bundles or secrets exit with an actionable error pointing at the platform Gateway.

For detailed help on any subcommand: `./g8e <command> --help`.

See also: [Operator](../architecture/operator.md), [Governance Gateway (g8eg)](../architecture/g8eg.md), [Tests](./tests.md).
