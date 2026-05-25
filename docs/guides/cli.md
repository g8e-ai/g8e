# CLI Guide

The g8e CLI is the primary operational interface for the Governance Gateway. It is a statically compiled Go binary that manages platform lifecycle, authentication, data queries, and testing.

## Architecture

g8e runs directly on the host without container-orchestration complexity. The CLI manages the Governance Gateway (the g8e Operator binary running in listen mode) as a separate process.

**Host runtime state** - All runtime data lives at `./.g8e/`: `data/`, `pki/`, `secrets/`, `logs/`.
**Credentials** - Authenticated commands use `~/.g8e/credentials`.

Command form: `./g8e <command> [subcommand] [options]`.

## Technical Invariants

1. **Zero Shell Scripts**: NO shell scripts are used for platform operations. All platform lifecycle, configuration, and administrative duties are handled by the unified Go binary.
2. **Service readiness** - The platform is not "ready" until the Gateway is running. The Go CLI checks process status before operations.
3. **Canonical wire format** - All client-facing interaction uses canonical JSON (protojson). Binary protobuf is reserved for internal storage.
4. **Fail-closed execution** - The CLI must never mask failures or proceed with missing trust material. Missing trust bundles or secrets exit with an actionable error pointing at the platform Gateway.

## Common Workflows

### First-Time Setup

```bash
# Bootstrap the platform with initial user and certificates
./g8e auth bootstrap

# Start the Governance Gateway
./g8e platform start
```

### Daily Operations

```bash
# Check platform status
./g8e platform status

# View logs
./g8e platform logs

# Restart the platform
./g8e platform restart
```

### Authentication

```bash
# Authenticate via device-link token
./g8e auth login

# Clear local session
./g8e auth logout
```

### Data Management

```bash
# List users
./g8e data users

# List operators
./g8e data operators

# Query document storage
./g8e data store --collection <collection>

# Query audit vault
./g8e data audit list --operator-session-id <session-id>
```

### Testing

```bash
# Run Gateway (g8eo) tests
./g8e test g8eo

# Run unit tests
./g8e test unit

# Run integration tests
./g8e test integration

# Run scenario tests
./g8e test scenario

# Run chaos tests
./g8e test chaos

# Run CI test suite
./g8e test ci
```

### Environment Variables

```bash
# List all variables
./g8e vars list

# Set a variable
./g8e vars set <key> <value>

# Get a variable
./g8e vars get <key>

# Unset a variable
./g8e vars unset <key>
```

### Security Validation

```bash
# Run security validation checks
./g8e security validate
```

## Command Reference

For detailed command help, use `--help`:

```bash
./g8e --help
./g8e platform --help
./g8e auth --help
./g8e data --help
./g8e test --help
./g8e security --help
./g8e vars --help
```

## Key Commands

- `setup` - Bootstrap dependencies and build services
- `platform` - Manage Gateway lifecycle (start, stop, status, restart, logs, settings, reset, clean)
- `auth` - Manage mTLS enrollment and sessions (bootstrap, login, logout)
- `data` - Administer substrate over mTLS (users, operators, device-links, settings, store, audit)
- `test` - Run test suites (unit, integration, g8eo, ci, chaos, scenario)
- `security` - Run security validation checks
- `vars` - Manage environment variables in `.g8e/.env`

## See Also

- [Operator](../architecture/operator.md)
- [Governance Gateway (g8eg)](../architecture/gateway.md)
- [Testing](../developer/testing.md)
