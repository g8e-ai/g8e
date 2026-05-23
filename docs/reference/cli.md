# CLI Reference

This document is auto-generated from the CLI help output. Do not edit manually.

## g8e Platform Commands

The `g8e` CLI is the primary interface for platform management.

### Available Commands

- `./g8e platform` - Gateway lifecycle management (start, stop, status, wipe, reset, clean)
- `./g8e apps` - Application layer management (start, stop, status for g8ee and other adapters)
- `./g8e operator` - Operator lifecycle and fleet deployment
- `./g8e data` - Data and infrastructure operations
- `./g8e security` - Security and PKI operations
- `./g8e protocol` - Protocol and constants management
- `./g8e testing` - Test execution and evaluation
- `./g8e setup` - Interactive setup wizard
- `./g8e login` - Authenticate the local CLI
- `./g8e test` - Run tests (default: g8eo)
- `./g8e evals` - Evaluation harness for AI benchmarks

For detailed command help, run `./g8e` and navigate the interactive menu.

## g8eo Operator Binary

The `g8e.operator` binary is the host-side Policy Execution Point (PEP).

```
Usage: g8e.operator [options]

Options:
  -k, --key <key>         API key (or set G8E_OPERATOR_API_KEY)
  -D, --device-token <tok> Device link token for operator deployment
  -e, --endpoint <host>     Operator endpoint: IP address of the Docker host running operator
      --trust-bundle <path> Path to trust bundle PEM file (default: .g8e/pki/hub-bundle.pem or fetch from /.well-known/g8e/pki/hub-bundle.pem)
      --working-dir <dir>   Working directory (default: directory operator was launched from)
                            All commands and data storage are anchored to this directory
      --http-port <port>    HTTPS port to dial for auth/bootstrap (default: 8440)
  -c, --cloud             Cloud Operator mode (for AWS/cloud CLI)
  -p, --provider <name>   Cloud provider: aws, gcp, azure
  -s, --local-storage     Store audit data locally instead of cloud (default: on)
                          When enabled, data is stored in ./.g8e/ relative to launch directory
  -l, --log <level>       Log level: info, error, debug (default: info)
  -G, --no-git            Disable ledger (git-backed file versioning)
      --heartbeat-interval <dur> Heartbeat interval (e.g. 60s, 2m); overrides the 30s default
  -v, --version           Show version

Gateway Mode (platform persistence + pub/sub broker):
  --doctrine                Gateway mode: L1 enforced, L2/L3 audited (default)
  --consensus               Gateway mode: L1/L2 enforced, L3 audited
  --notary                  Gateway mode: L1/L2/L3 strictly enforced
  --http-listen-port <port>   HTTPS port for mTLS API (default: 8440)
  --bootstrap-listen-port <port> Bootstrap TLS port for device-link enrollment (default: 8441)
  --public-listen-port <port> Public browser/BYO bootstrap port (default: 8442)
  --data-dir <dir>            Data directory for SQLite (default: .g8e/data in working directory)
  --pki-dir <dir>             Directory for TLS certificates (default: .g8e/pki)
  --secrets-dir <dir>         Directory for platform secrets (default: .g8e/secrets)
  --passkey-rp-id <id>        RP ID for passkey operations (default: localhost)
  --passkey-rp-name <name>    RP Name for passkey operations (default: g8e)

Vault Management:
  --rekey-vault           Re-encrypt vault with new API key
  --old-key <key>         Old API key (required for --rekey-vault)
  --verify-vault          Verify vault integrity
  --reset-vault           Reset vault (DESTROYS ALL DATA)

OpenClaw Node Host Mode:
  --openclaw              Connect to an OpenClaw Gateway as a node host
  --openclaw-url <url>    OpenClaw Gateway WebSocket URL (e.g. ws://localhost:18789)
  --openclaw-token <tok>  Auth token (or set OPENCLAW_GATEWAY_TOKEN)
  --openclaw-node-id <id> Node ID advertised to the Gateway (default: hostname)
  --openclaw-name <name>  Display name shown in OpenClaw UI (default: node ID)
```

