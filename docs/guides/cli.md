# CLI Reference

Last Updated: 2026-06-25
Version: v1.2.1

This reference documents the g8e CLI commands for managing the g8e Gateway, g8e Operator, and platform setup.

## g8e Root Help
```
g8e is a zero-trust execution platform for agentic infrastructure.
The CLI manages the g8e Gateway (g8eg), g8e Operator (g8eo), and platform setup.

Usage:
  g8e [command]

Available Commands:
  auth                   Authentication and session management
  gateway                Manage the g8e Gateway lifecycle (alias: gw)
  mcp                    MCP protocol operations (stdio transport with full governance)
  operator               Manage Operator instances
  vault                  Manage the encryption vault
  test                   Run test suites (unit, integration, e2e, lint, agent-harness, chaos)
  demos                  Manage g8e demo environments
  audit                  Run audit reports for compliance
  report                 Generate CSV evidence reports from all persistent stores
  swagger                Manage Swagger/OpenAPI documentation
  agent-harness  Universal agent harness for a real g8e Gateway/Operator
  help                   Help about any command

Flags:
  -h, --help      help for g8e
  -v, --version   version for g8e

Use "g8e [command] --help" for more information about a command.
```


## gateway (gw)
```
Gateway lifecycle commands for starting, stopping, and checking the status of the g8e Gateway.

Usage:
  g8e gateway [command]
  g8e gw [command]

Available Commands:
  start       Start the g8e Gateway
  stop        Stop the g8e Gateway
  status      Check Gateway health and status
  restart     Restart the g8e Gateway
  logs        View Gateway logs
  settings    Manage Gateway settings
  reset       Reset Gateway data and secrets (preserves CA)
  clean       Destructively remove all Gateway state
  data        Administer the local platform over mTLS
  security    Security validation checks

Flags:
  -h, --help   help for gateway

Use "g8e gateway [command] --help" for more information about a command.
```

### gateway start
```
Start the g8e Gateway. Bootstraps the stateless gateway with PKI, persistence, and pub/sub. The gateway must be running before any client can authenticate.

Usage:
  g8e gateway start [flags]
  g8e gw start [flags]

Flags:
      --cert-mode string         Certificate mode: full (all hostnames/IPs), localhost (only localhost)
      --data-dir string          Data directory for SQLite database (default: .g8e/data in working directory)
  -h, --help                     help for start
      --http-port int            HTTP port for bootstrap and MCP (default: from constants.Ports.OperatorHttp)
      --https-port int           HTTPS port for mTLS API (default: from constants.Ports.OperatorHttps)
      --log string               Log level: info, error, debug (default "info")
      --passkey-rp-id string     RP ID for passkey operations (default: localhost)
      --passkey-rp-name string   RP Name for passkey operations (default: g8e)
      --pki-dir string           Directory for TLS certificates (default: .g8e/pki)
      --posture string           Gateway posture: doctrine (L1 enforced, L2/L3 audited), consensus (L1/L2 enforced, L3 audited), notary (L1/L2/L3 strictly enforced) (default "doctrine")
      --rate-limit-burst int     Gateway rate limit burst size
      --rate-limit-rps float     Gateway requests per second limit (set to 0 to disable)
      --secrets-dir string       Directory for platform secrets (default: .g8e/secrets)
      --vault-dir string         Directory for vault data (default: .g8e/vault)
      --vault-key string         Path to vault private key (default: .g8e/secrets/vault.key)
      --vault-require-unlock     Require vault to be unlocked at startup (fail if vault cannot be unlocked)
  -f, --follow                   Follow log output after starting (like tail -f)
```

When `--cert-mode full` is selected, the CLI detects network identity once, writes it to a temporary JSON file in the runtime directory, and passes that file to the Gateway subprocess. `--cert-mode localhost` continues to use loopback-only identities, including IPv6 localhost when available.

**Posture Persistence:** The gateway posture is persisted in `.g8e/pids/operator.posture` on startup. When using `gateway restart`, the current posture is read from this file and preserved. If the file is missing or corrupted, the gateway defaults to `doctrine` posture. Valid posture values are `doctrine`, `consensus`, and `notary`.

### gateway stop
```
Stop the g8e Gateway

Usage:
  g8e gateway stop [flags]
  g8e gw stop [flags]

Flags:
  -h, --help   help for stop
```

### gateway status
```
Check Gateway health and status

Usage:
  g8e gateway status [flags]
  g8e gw status [flags]

Flags:
  -h, --help   help for status
```

### gateway restart
```
Restart the g8e Gateway

Usage:
  g8e gateway restart [flags]
  g8e gw restart [flags]

Flags:
  -h, --help   help for restart
```

### gateway logs
```
View Gateway logs

Usage:
  g8e gateway logs [flags]
  g8e gw logs [flags]

Flags:
  -f, --follow   Follow log output (like tail -f)
  -h, --help     help for logs
```

### gateway settings
```
Manage Gateway settings

Usage:
  g8e gateway settings [flags]
  g8e gw settings [flags]

Flags:
  -h, --help   help for settings
```

### gateway reset
```
Reset Gateway data and secrets (preserves CA)

Usage:
  g8e gateway reset [flags]

Flags:
      --force   Skip confirmation prompt
  -h, --help    help for reset
      --y       Skip confirmation prompt (shorthand)
      --yes     Skip confirmation prompt (shorthand)
```

### gateway clean
```
Destructively remove all Gateway state

Usage:
  g8e gateway clean [flags]

Flags:
      --force   Skip confirmation prompt
  -h, --help    help for clean
      --y       Skip confirmation prompt (shorthand)
      --yes     Skip confirmation prompt (shorthand)
```

## auth
```
Authentication and session management

Usage:
  g8e auth [command]

Available Commands:
  enroll          Enroll CLI with the running Gateway and register a passkey
  logout          Clear local Operator session and credentials
  enroll-windows  Enroll via Windows Certificate Store (Windows only - advanced)
  approve         Approve a suspended L3 transaction with CLI signature

Flags:
  -h, --help   help for auth

Use "g8e auth [command] --help" for more information about a command.
```

### auth enroll
```
Enroll CLI with the running Gateway via CSR-based enrollment, then register a passkey for secure authentication. Generates client keypairs, submits CSRs to the Gateway's CA, saves signed mTLS credentials, and opens a browser to register a WebAuthn/FIDO2 passkey. The Gateway must already be running (use './g8e gw start' first). On Windows, this automatically enrolls via Windows Certificate Store for passkey authentication.

Usage:
  g8e auth enroll [flags]

Flags:
  -h, --help   help for enroll
```

### auth logout
```
Clear local Operator session and credentials

Usage:
  g8e auth logout [flags]

Flags:
  -h, --help   help for logout
```

### auth enroll-windows
```
Enroll via Windows Certificate Store (Windows only - advanced). Generate an ECDSA P-256 keypair in the Windows Certificate Store, submit a CSR to the Gateway, and import the signed certificate. Chrome/Edge will automatically present this cert. Use --tpm for TPM-backed keys via Windows Hello for Business.

NOTE: This is now handled automatically by 'g8e auth enroll' on Windows. This command is for advanced use cases or manual re-enrollment.

Usage:
  g8e auth enroll-windows [flags]

Flags:
      --tpm   Use TPM-backed key via Windows Hello for Business
  -h, --help   help for enroll-windows
```

### auth approve
```
Approve a suspended L3 transaction with CLI signature. Approve a suspended transaction by signing the transaction hash with the CLI private key and submitting the cryptographic proof to the Gateway.

Usage:
  g8e auth approve <transaction_hash> [flags]

Flags:
  -h, --help   help for approve
```

### gateway data
```
Data management commands for users, operators, settings, and audit.

Usage:
  g8e gateway data [command]

Available Commands:
  users        Manage user accounts
  operators    Manage Operator instances
  settings     Manage Gateway settings
  store        Manage document storage
  audit        Query audit vault

Flags:
  -h, --help   help for data

Use "g8e gateway data [command] --help" for more information about a command.
```

#### gateway data users
```
Manage user accounts

Usage:
  g8e gateway data users [flags]

Flags:
  -h, --help   help for users
```

#### gateway data operators
```
Manage Operator instances

Usage:
  g8e gateway data operators [flags]

Flags:
  -h, --help   help for operators
```

#### gateway data settings
```
Manage Gateway settings

Usage:
  g8e gateway data settings [flags]

Flags:
  -h, --help   help for settings
```

#### gateway data store
```
Manage document storage

Usage:
  g8e gateway data store [flags]

Flags:
      --collection string    Collection name
      --document-id string   Document ID (omit to list collection)
  -h, --help                 help for store
```

#### gateway data audit
```
Query audit vault

Usage:
  g8e gateway data audit [command]

Available Commands:
  list        List audit events for a session
  summary     Show audit event summary by type

Flags:
  -h, --help   help for audit

Use "g8e gateway data audit [command] --help" for more information about a command.
```

##### gateway data audit list
```
List audit events for a session

Usage:
  g8e gateway data audit list [flags]

Flags:
  -h, --help                         help for list
      --limit int                    Limit number of events (default 100)
      --operator-session-id string   Operator session ID
```

##### gateway data audit summary
```
Show audit event summary by type

Usage:
  g8e gateway data audit summary [flags]

Flags:
  -h, --help                         help for summary
      --operator-session-id string   Filter by Operator session ID
```

### gateway security
```
Security validation checks

Usage:
  g8e gateway security [command]

Available Commands:
  validate    Run security validation checks
  pki         PKI management

Flags:
  -h, --help   help for security

Use "g8e gateway security [command] --help" for more information about a command.
```

#### gateway security validate
```
Run security validation checks

Usage:
  g8e gateway security validate [flags]

Flags:
  -h, --help                 help for validate
      --pki-dir string       PKI directory (default: .g8e/pki)
      --secrets-dir string   Secrets directory (default: .g8e/secrets)
```

#### gateway security pki
```
PKI management

Usage:
  g8e gateway security pki [command]

Available Commands:
  enroll      Enroll an operator with the Gateway via CSR

Flags:
  -h, --help   help for pki

Use "g8e gateway security pki [command] --help" for more information about a command.
```

##### gateway security pki enroll
```
Enroll an operator with the Gateway via CSR. Generate a CSR and enroll with the Gateway to obtain Operator mTLS certificates.

Usage:
  g8e gateway security pki enroll [flags]

Flags:
  -e, --endpoint string     Gateway IP address (e.g., 192.168.1.62)
  -h, --help                help for enroll
      --output-dir string   Output directory for certificates (default: project root)
```

## test
```
Run test suites (unit, integration, e2e, lint, agent-harness, chaos)

Usage:
  g8e test [command]

Available Commands:
  unit        Run Tier 1 (Unit) tests
  integration Run Tier 2 (In-Process Integration) tests
  e2e         Run Tier 3 (Live Platform E2E) tests
  coverage    Run tests with coverage report
  lint        Run linting and quality checks
  agent-harness    Universal agent harness for a real g8e Gateway/Operator
  chaos       Generate realistic governance events against the local g8e audit stack
  summary     View chaos test summary from test vault

Flags:
  -h, --help   help for test

Use "g8e test [command] --help" for more information about a command.
```

### test unit
```
Run Tier 1 (Unit) tests. These tests use mocks/stubs and have no external dependencies (no files, network, or DB).

Usage:
  g8e test unit [flags]

Flags:
  -h, --help   help for unit
```

### test integration
```
Run Tier 2 (In-Process Integration) tests. These tests run the gateway in-process against real on-disk SQLite databases, local PKI generation, and local pubsub.

Usage:
  g8e test integration [flags]

Flags:
  -h, --help   help for integration
```


### test e2e
```
Run Tier 3 (Live Platform E2E) tests. These tests require a running g8e gateway and authenticated CLI session.

Usage:
  g8e test e2e [flags]

Flags:
  -h, --help   help for e2e
```


### test coverage
```
Run tests with coverage report and enforce a minimum coverage threshold (60%). Use --pkg flag to test a specific package, --verbose for detailed output.

Usage:
  g8e test coverage [flags]

Flags:
  -h, --help        help for coverage
      --pkg string  Specific package to test (e.g., ./internal/services/auth)
      --verbose     Verbose output
```

### test lint
```
Run linting and quality checks using golangci-lint with modern Go 1.26.3 best practices. This includes staticcheck, govet, and additional linters for bug prevention, security, and code quality.

Usage:
  g8e test lint [flags]

Flags:
  -h, --help   help for lint
```

### test agent-harness
```
Universal agent harness for a real g8e Gateway/Operator. Impersonates arbitrary AI tools and agents against a REAL g8e Gateway + Operator, exercising the full protocol surface (MCP, A2A, A2A protobuf, and official governance envelopes with mock consensus + principal signing), then audits every result against the Operator's signed receipts.

The agent harness is a protocol compliance verifier that records every HTTP exchange with detailed metadata (request/response bodies, latency, status codes) and cross-references against the Operator's signed receipts. The ONLY fiction is the client identity, the Gateway and Operator are real infrastructure.

Usage:
  g8e test agent-harness [command]

Available Commands:
  list        List available scenarios
  run         Run scenarios against a real Gateway/Operator
  audit       Audit signed receipts from the Operator

Flags:
  -h, --help   help for agent-harness

Use "g8e test agent-harness [command] --help" for more information about a command.
```

The `agent-harness` command is also available as a top-level command (`g8e agent-harness`) with identical subcommands and flags.

#### test agent-harness list
```
List available scenarios

Usage:
  g8e test agent-harness list [flags]

Flags:
  -h, --help   help for list
```

#### test agent-harness run
```
Run scenarios against a real Gateway/Operator

Usage:
  g8e test agent-harness run [flags] [scenario ...]

Flags:
      --config string            JSON config overlay
      --mtls-url string          Gateway mTLS surface
      --public-url string        Gateway public surface for OOB approve
      --cert string              client cert PEM
      --key string               client key PEM
      --ca string                gateway CA bundle PEM
      --api-key string           operator API key for MCP/A2A surface
      --operator-session string   scope audit to a specific Operator session
      --insecure                 skip TLS verify (local dev only)
      --out string               report output dir
      --l3-mode string           mock|suspend
      --ensemble int             mock consensus voters (default 3)
      --verbose                  echo each request/response
      --phase string             doctrine|notary|all (default "all")
  -h, --help                     help for run
```

#### test agent-harness audit
```
Audit signed receipts from the Operator

Usage:
  g8e test agent-harness audit [flags]

Flags:
      --config string            JSON config overlay
      --mtls-url string          Gateway mTLS surface
      --public-url string        Gateway public surface
      --cert string              client cert PEM
      --key string               client key PEM
      --ca string                gateway CA bundle PEM
      --api-key string           operator API key
      --operator-session string   operator session id
      --insecure                 skip TLS verify
      --out string               report output dir
  -h, --help                     help for audit
```

### test chaos
```
Generate realistic governance events against the local g8e audit stack. Bypasses network/TLS by driving the TransactionVerifier + Actuator stack directly in-process, which is the same path exercised by the live Operator when payloads arrive over pub/sub.

Distribution:
  70%  Good Actor  - valid sig, safe intent (FS_LIST)       -> EXECUTED
  20%  Prompt Inj  - valid sig, L1 forbidden cmd (sudo/rm)  -> REJECTED (L1)
  10%  MitM        - corrupted transaction hash              -> REJECTED (hash mismatch)

Usage:
  g8e test chaos [flags]

Flags:
      --count int      number of payloads to fire (default 100)
      --data-dir string audit vault data dir (default: <project-root>/.g8e/test-vault/<timestamp>)
  -h, --help            help for chaos
      --pki-dir string  PKI dir for trusted_signers (default: <cwd>/.g8e/pki)
```

### test summary
```
View aggregated chaos test results from the test vault database. This queries the chaos_events table across all test runs in the test vault directory.

Usage:
  g8e test summary [flags]

Flags:
  -h, --help   help for summary
```

## operator
```
Manage Operator instances

Usage:
  g8e operator [command]

Available Commands:
  list        List all Operator instances
  cp          Copy the operator binary to a target location
  scp         Copy the operator binary to a remote host using scp
  deploy      Deploy the operator binary to remote hosts and start it
  stream      Stream and execute the operator on remote hosts via SSH

Flags:
  -h, --help   help for operator

Use "g8e operator [command] --help" for more information about a command.
```

### operator list
```
List all Operator instances currently connected to the Gateway.

Usage:
  g8e operator list [flags]

Flags:
  -h, --help   help for list
```

### operator cp
```
Copy the operator binary to a target location. If a directory is provided, the binary will be copied with its default name. If a filename is provided, the binary will be copied with that name.

Usage:
  g8e operator cp <target> [flags]

Flags:
  -h, --help   help for cp
```

### operator scp
```
Copy the operator binary to a remote host using scp. Supports common scp flags. If the target path is a directory, the binary will be copied with its default name. Use --prompt to interactively configure scp options.

Usage:
  g8e operator scp <user@host:path> [flags]

Flags:
  -P, --port int              Port to connect to on the remote host
  -i, --identity string       Selects the file from which the identity (private key) for public key authentication is read
  -r, --recursive             Recursive copy (not applicable for single file, but included for compatibility)
  -p, --preserve              Preserves modification times, access times, and modes from the source file
  -v, --verbose               Verbose mode
  -C, --compression           Enable compression
      --prompt                 Prompt for scp options interactively
  -h, --help                  help for scp
```

### operator deploy
```
Deploy the operator binary to remote hosts via SSH and start it in the background. Uses your existing SSH config for authentication. Requires 'g8e auth enroll' first.

Usage:
  g8e operator deploy [flags]

Flags:
      --hosts string            Comma-separated list of hosts to deploy to (required)
  -P, --port int              SSH port to connect to on remote hosts
  -i, --identity string       SSH identity file (private key)
      --background              Start operator in background after deployment
  -h, --help                  help for deploy
```

### operator stream
```
Stream the operator binary via SSH and execute it directly on remote hosts without copying. This is useful for quick deployments or air-gapped scenarios. Requires 'g8e auth enroll' first.

Usage:
  g8e operator stream [flags]

Flags:
      --hosts string            Comma-separated list of hosts to stream to (required)
  -P, --port int              SSH port to connect to on remote hosts
  -i, --identity string       SSH identity file (private key)
  -h, --help                  help for stream
```

## vault
```
Manage the encryption vault

Usage:
  g8e vault [command]

Available Commands:
  init        Initialize a new encryption vault
  unlock      Unlock the encryption vault
  rekey       Re-key the vault with a new private key
  status      Show vault status
  reset       Destroy the vault and all encrypted data
  export      Export the vault key
  import      Import a vault key

Flags:
  -h, --help   help for vault

Use "g8e vault [command] --help" for more information about a command.
```

### vault init
```
Initialize a new encryption vault. Generate a new encryption vault with a random key. The key is saved to the specified key path.

Usage:
  g8e vault init [flags]

Flags:
      --vault-dir string   Vault directory (default: .g8e/vault)
      --key-path string    Path to save the vault key
  -h, --help                help for init
```

### vault unlock
```
Unlock an existing vault using the private key.

Usage:
  g8e vault unlock [flags]

Flags:
      --vault-dir string   Vault directory (default: .g8e/vault)
      --key-path string    Path to the vault key
  -h, --help                help for unlock
```

### vault rekey
```
Re-encrypt the vault's DEK with a new private key. Both old and new keys are required.

Usage:
  g8e vault rekey [flags]

Flags:
      --vault-dir string       Vault directory (default: .g8e/vault)
      --key-path string        Path to the current vault key
      --new-key-path string    Path to save the new vault key (default: <key-path>.new)
  -h, --help                    help for rekey
```

### vault status
```
Display whether the vault is initialized and unlocked.

Usage:
  g8e vault status [flags]

Flags:
      --vault-dir string   Vault directory (default: .g8e/vault)
  -h, --help                help for status
```

### vault reset
```
Reset the vault completely. This is a destructive operation that makes all encrypted data unrecoverable.

Usage:
  g8e vault reset [flags]

Flags:
      --vault-dir string   Vault directory (default: .g8e/vault)
      --confirm            Skip interactive confirmation (dangerous)
  -h, --help                help for reset
```

### vault export
```
Export the vault private key in hex format. Use with extreme caution.

Usage:
  g8e vault export [flags]

Flags:
      --key-path string    Path to the vault key
  -h, --help                help for export
```

### vault import
```
Import a vault private key from hex string or stdin.

Usage:
  g8e vault import [flags]

Flags:
      --key-path string    Path to save the vault key
      --key-hex string     Vault key as hex string (if not provided, reads from stdin)
  -h, --help                help for import
```


## audit
```
Run audit reports for compliance evidence, signed receipts, and event logs.

Usage:
  g8e audit [command]

Available Commands:
  receipts    List signed receipts from the running Gateway
  export      Export the full receipts bundle for archival
  report      Generate a compliance report (JSON + Markdown)
  events      Query raw audit events from the Gateway audit store
  summary     Aggregate audit events and receipts by type

Flags:
  -h, --help   help for audit

Use "g8e audit [command] --help" for more information about a command.
```

### audit receipts
```
List signed receipts from the running Gateway. Auto-discovers session ID if omitted.

Usage:
  g8e audit receipts [flags]

Flags:
  -h, --help             help for receipts
      --json             Raw JSON output
      --session string   Operator session ID (auto-discovers if omitted)
      --tx-id string     Get a single receipt by transaction ID
```

### audit export
```
Export the full receipts bundle for archival.

Usage:
  g8e audit export [flags]

Flags:
  -h, --help             help for export
      --out string       Output file path (default "./receipts-export.json")
      --session string   Operator session ID
```

### audit report
```
Generate a comprehensive compliance report (JSON + Markdown).

Usage:
  g8e audit report [flags]

Flags:
  -h, --help             help for report
      --out string       Output directory (default "./reports")
      --session string   Operator session ID
```

### audit events
```
Query raw audit events from the Gateway audit store.

Usage:
  g8e audit events [flags]

Flags:
  -h, --help             help for events
      --json             Raw JSON output
      --limit int        Max rows (default 100)
      --session string   Filter by operator session ID (shows all if omitted)
```

### audit summary
```
Aggregate audit events and receipts by type.

Usage:
  g8e audit summary [flags]

Flags:
  -h, --help             help for summary
      --session string   Filter by Operator session ID
```

## demos
```
Manage Docker Compose demo environments for org-specific g8e deployments. Each org environment is hermetically sealed with no shared state, volumes, or cross-org dependencies.

Usage:
  g8e demos [command]

Available Commands:
  list        List available demo environments
  start       Start a demo environment
  stop        Stop a demo environment
  status      Show status of a demo environment
  clean       Remove containers, volumes, and networks for a demo environment
  reset       Clean and restart a demo environment
  run         Run demo scenarios
  audit       View audit logs and ledger history

Flags:
  -h, --help   help for demos

Use "g8e demos [command] --help" for more information about a command.
```

### demos list
```
List available demo environments

Usage:
  g8e demos list [flags]

Flags:
  -h, --help   help for list
```

### demos start
```
Start a demo environment

Usage:
  g8e demos start <org> [flags]

Flags:
  -h, --help   help for start
```

### demos stop
```
Stop a demo environment

Usage:
  g8e demos stop <org> [flags]

Flags:
  -h, --help   help for stop
```

### demos status
```
Show status of a demo environment

Usage:
  g8e demos status <org> [flags]

Flags:
  -h, --help   help for status
```

### demos clean
```
Remove containers, volumes, and networks for a demo environment

Usage:
  g8e demos clean <org> [flags]

Flags:
  -h, --help   help for clean
```

### demos reset
```
Clean and restart a demo environment

Usage:
  g8e demos reset <org> [flags]

Flags:
  -h, --help   help for reset
```

### demos run
```
Run demo scenarios. Omit the scenario number to run all scenarios in sequence.

Available scenarios:
  healthcare: 1-4
    1 - Authorized Agent Submits a FHIR PA Request
    2 - Gold Card Auto-Approval
    3 - SLA Breach and OHA Reporting
    4 - Bad Actor PHI Exfiltration Blocked
  gov: 1
    1 - CUI Exfiltration Attempt Blocked
  finance: 1
    1 - Unauthorized Trade Blocked
  secure-data: 1-3
    1 - Governed Migration with Chain-of-Custody Receipts
    2 - Connector Bypass Attempt Blocked
    3 - Cross-Tenant Leak Doctrine Triggered

Usage:
  g8e demos run <org> [scenario] [flags]

Flags:
  -h, --help   help for run
```

### demos audit
```
View audit logs and ledger history for a demo environment.
Without an action, it prints a summary of available audit resources.

Actions:
  logs              Tail the observability logs
  gateway-db        Open the gateway audit database (SQLite)
  operator-db       Open the operator audit database (SQLite)
  ledger-log        View the git ledger log
  ledger-files      List all files in the git ledger
  ledger-history <file> View git history for a specific file
  ledger-show <hash> View a specific git commit diff
  vault             Open the execution vault database (SQLite)

Usage:
  g8e demos audit <org> [action] [flags]

Flags:
  -h, --help   help for audit
```

## mcp
```
MCP protocol operations (stdio transport with full governance). Run g8e as an MCP server using stdio transport for local agent integration. All MCP calls are proxied through the gateway with full L1-L5 governance enforcement.

Usage:
  g8e mcp [command]

Available Commands:
  stdio       Run MCP stdio server with full L1-L5 governance (proxies to gateway)
  agent       Agent integration commands for popular AI coding tools

Flags:
  -h, --help   help for mcp

Use "g8e mcp [command] --help" for more information about a command.
```

### mcp stdio
```
Run MCP stdio server with full L1-L5 governance (proxies to gateway). Run as an MCP stdio server that proxies all requests to the running gateway over mTLS with a bound CLI session. Every tool call passes through the L1-L5 governance pipeline. HTTP is never used for proxy traffic; it is reserved for CA bundle discovery and health checks only.

This command is launched automatically by 'g8e mcp agent run'. When invoked directly the CLI session is loaded from disk (bootstrapping enrollment if needed).

Usage:
  g8e mcp stdio [flags]

Flags:
  -h, --help   help for stdio
```

### mcp agent
```
Agent integration commands for popular AI coding tools. Configure and integrate g8e with popular AI agent binaries (Claude, Cursor, Devin, etc.) for seamless MCP tool access.

Usage:
  g8e mcp agent [command]

Available Commands:
  list        List supported agent binaries
  show        Print MCP client configuration for the Gateway
  run         Govern any MCP server via g8e reverse proxy

Flags:
  -h, --help   help for agent
```

#### mcp agent list
```
List all popular AI agent binaries that g8e supports for MCP integration.

Usage:
  g8e mcp agent list [flags]

Flags:
  -h, --help   help for list
```

#### mcp agent show
```
Print MCP client configuration for the Gateway. Displays configurations side-by-side for g8e.local (mTLS), IP Address (mTLS), and Stdio Transport.

Usage:
  g8e mcp agent show <agent> [flags]

Flags:
  -h, --help   help for show
```

#### mcp agent run
```
Launch an AI agent or wrap an MCP server with g8e governance.

LAUNCH AN AGENT (one command does everything):

  g8e mcp agent run claude       Start the g8e gateway (if not already running),
                                  perform CLI auth, then launch Claude with native
                                  tools disabled so ALL I/O must go through g8e MCP.
                                  Every action is audited at L1-L5. No other MCP
                                  servers are reachable.

  g8e mcp agent run cursor        Launch Cursor IDE with g8e MCP config written
                                  to ~/.cursor/mcp.json

  g8e mcp agent run devin         Launch Devin IDE with g8e MCP config written
                                  to ~/.codeium/windsurf/mcp_config.json

  g8e mcp agent run aider         Launch Aider with g8e MCP config written to
                                  .aider.conf.yml in the current directory

  g8e mcp agent run continue      Launch Continue CLI with g8e MCP config

  Extra args are forwarded to the agent:
    g8e mcp agent run claude -- -p "fix the failing tests"

WRAP AN EXTERNAL MCP SERVER (governance reverse proxy):

  g8e mcp agent run -- npx -y @modelcontextprotocol/server-filesystem /home/user
  g8e mcp agent run --url http://localhost:3000

  Intercepts all tools/call requests, screens them through L1 doctrine
  (MITRE ATT&CK threat detection), and blocks violations before forwarding.

AUDIT TRAIL:
  When launching an agent with 'g8e mcp agent run', the agent is automatically
  enrolled as an external app identity (SPIFFE ID: spiffe://g8e.local/app/<agent-name>).
  All MCP tool calls are recorded in the audit vault with this app identity,
  enabling per-agent audit trails separate from human operator activity.

  Query audit events for a specific agent:
    g8e gateway data audit list --operator-session-id spiffe://g8e.local/app/claude
    g8e gateway data audit summary --operator-session-id spiffe://g8e.local/app/claude

  View all audit events:
    g8e gateway data audit summary

DELEGATED CREDENTIAL MODEL:
  g8e uses a delegated credential model for agent identity. When an agent is launched,
  it receives a short-lived mTLS certificate that carries both identities:
  - App SPIFFE ID: spiffe://g8e.local/app/<agent-name> (the agent's policy identity)
  - Requestor User ID: spiffe://g8e.local/user/<id> (the human who launched the agent)

  Both identities are cryptographically bound in the certificate's URI SANs and presented
  at the TLS handshake. No trusted identity headers are used; the certificate IS the
  session. Every governed transaction includes both identities in the signed hash,
  ensuring end-to-end identity correctness and auditability.

Usage:
  g8e mcp agent run [<agent>] [--url <url>] [-- <command> [args...]] [flags]

Flags:
  -h, --help         help for run
      --url string   URL of the downstream HTTP MCP server
```

## Agent Integration

### Quick Start

1. Start the gateway:
   ```bash
   ./g8e gateway start
   ```

2. Authenticate your CLI:
   ```bash
   ./g8e auth enroll
   ```

3. List supported agent binaries:
   ```bash
   ./g8e mcp agent list
   ```

4. Show MCP configuration for your agent:
   ```bash
   ./g8e mcp agent show claude
   ./g8e mcp agent show cursor
   ./g8e mcp agent show devin
   ```

5. Copy the generated JSON configuration to your agent's MCP settings file.

### Supported Agent Binaries

- **claude** - Anthropic Claude Desktop / Claude Code
- **codex** - OpenAI Codex AI coding assistant
- **cursor** - Cursor AI IDE
- **devin** - Devin AI IDE (formerly Windsurf)
- **vscode** - Visual Studio Code with MCP extension
- **continue** - Continue.dev AI coding assistant (alias: cn)
- **aider** - Aider AI pair programmer
- **codeium** - Codeium AI assistant
- **tabby** - Tabby AI autocomplete
- **ollama** - Ollama local LLM runner
- **gemini** - Google Gemini CLI
- **goose** - Goose AI coding assistant
- **generic** - Generic MCP-compatible agent

### Configuration Example

For Claude Desktop, the configuration command displays all available connection options including g8e.local (mTLS), IP Address (mTLS), Plain HTTP, and Stdio Transport. Choose the appropriate configuration based on your environment.

### L3 Approval Flow

When a tool requires L3 approval, g8e will:
1. Automatically open your browser to the approval URL
2. Wait for you to authorize via WebAuthn
3. Retry the tool call automatically
4. Return the result to the tool

### Governance Proxy for Third-Party MCP Servers

To wrap any external MCP server in g8e's L1 doctrine without running the full gateway, use `agent run`. It intercepts all tool calls and screens them through MITRE ATT&CK threat detection before forwarding.

```bash
# Wrap a filesystem MCP server subprocess
./g8e mcp agent run -- npx -y @modelcontextprotocol/server-filesystem /home/user

# Wrap an HTTP MCP server
./g8e mcp agent run --url http://localhost:3000
```

Register the proxy as the MCP server in your agent config. The downstream tool's full `tools/list` and all pass-through methods are preserved; only `tools/call` is intercepted for governance.

For full L1-L5 governance (L2 consensus, L3 human approval via WebAuthn), start the gateway and use `g8e mcp stdio`.

### Manual MCP Configuration

For tools that don't support the agent wrapper, use the agent show command:

```bash
./g8e mcp agent show claude    # Show all MCP client configurations for Claude
./g8e mcp stdio               # Run as MCP stdio proxy to Gateway with full governance
```

## swagger
```
Manage Swagger/OpenAPI documentation for the g8e Gateway API. Commands for generating, serving, and validating Swagger/OpenAPI documentation.

Usage:
  g8e swagger [command]

Available Commands:
  init        Generate Swagger documentation from code annotations
  serve       Serve Swagger UI for API documentation
  validate    Validate Swagger/OpenAPI specification

Flags:
  -h, --help   help for swagger

Use "g8e swagger [command] --help" for more information about a command.
```

### swagger init
```
Generate Swagger/OpenAPI documentation by scanning Go code for Swagger annotations. Uses the `swag` CLI tool to parse annotations and generate docs. The `swag` binary must be installed (`go install github.com/swaggo/swag/cmd/swag@latest`).

Usage:
  g8e swagger init [flags]

Flags:
      --dir string      Directory to search for Swagger annotations (default: cmd/operator,internal/services/gateway)
      --output string   Output directory for generated docs (default: internal/services/gateway/docs)
  -h, --help            help for init
```

The generated documentation includes:
- `swagger.json` - OpenAPI 2.0 specification in JSON format
- `swagger.yaml` - OpenAPI 2.0 specification in YAML format

### swagger serve
```
Start a local HTTP server to serve the Swagger UI for viewing and testing the API documentation. This command provides instructions for serving the Swagger UI either through the running gateway or via external tools.

Usage:
  g8e swagger serve [flags]

Flags:
      --host string   Host to bind to (default: localhost)
      --port int      Port to serve Swagger UI on (default: 8081)
  -h, --help          help for serve
```

The Swagger UI is also available directly from the running gateway at:
- `https://localhost:8443/swagger/index.html`

### swagger validate
```
Validate the generated Swagger/OpenAPI specification for errors and compliance. This command checks the swagger.json file for correctness using available validation tools.

Usage:
  g8e swagger validate [flags]

Flags:
      --file string   Path to Swagger spec file (default: internal/services/gateway/docs/swagger.json)
  -h, --help          help for validate
```

If no validation tool is installed, the command will suggest installing one of:
- `npm install -g @apidevtools/swagger-cli`
- `go install github.com/go-swagger/go-swagger/cmd/swagger@latest`

## report
```
Generate CSV evidence reports from all persistent stores. Generate flat, deterministic CSV files from every g8e persistent store. Each file contains one record type with cryptographic proof fields. A verification pass independently re-validates receipt signatures, the commitment hash chain, and the git merkle root.

Usage:
  g8e report [command]

Available Commands:
  all         Export all stores to CSV and run verification
  verify      Run verification checks and write verification_summary.csv

Flags:
  -h, --help   help for report

Use "g8e report [command] --help" for more information about a command.
```

### report all
```
Export all stores to CSV and run verification.

Usage:
  g8e report all [flags]

Flags:
      --data-dir string     Data directory (default: .g8e/data)
  -h, --help                help for all
      --ledger-dir string   Ledger base directory (default: <runtime-dir>/ledger)
      --out string          Output directory (default: reports/<timestamp>)
      --runtime-dir string  Runtime directory (default: .g8e)
```

### report verify
```
Run verification checks and write verification_summary.csv.

Usage:
  g8e report verify [flags]

Flags:
      --data-dir string     Data directory (default: .g8e/data)
  -h, --help                help for verify
      --ledger-dir string   Ledger base directory (default: <runtime-dir>/ledger)
      --out string          Output directory (default: reports/<timestamp>)
      --runtime-dir string  Runtime directory (default: .g8e)
```

## agent-harness
```
Universal agent harness for a real g8e Gateway/Operator. Impersonates arbitrary AI tools and agents against a REAL g8e Gateway + Operator, exercising the full protocol surface (MCP, A2A, A2A protobuf, and official governance envelopes with mock consensus + principal signing), then audits every result against the Operator's signed receipts.

Usage:
  g8e agent-harness [command]

Available Commands:
  list        List available scenarios
  run         Run scenarios against a real Gateway/Operator
  audit       Audit signed receipts from the Operator

Flags:
  -h, --help   help for agent-harness

Use "g8e agent-harness [command] --help" for more information about a command.
```

The `agent-harness` command is also available as a subcommand of `test` (`g8e test agent-harness`) with identical subcommands and flags.

### agent-harness list
```
List available scenarios

Usage:
  g8e agent-harness list [flags]

Flags:
  -h, --help   help for list
```

### agent-harness run
```
Run scenarios against a real Gateway/Operator

Usage:
  g8e agent-harness run [flags] [scenario ...]

Flags:
      --config string            JSON config overlay
      --mtls-url string          Gateway mTLS surface
      --public-url string        Gateway public surface for OOB approve
      --cert string              client cert PEM
      --key string               client key PEM
      --ca string                gateway CA bundle PEM
      --api-key string           operator API key for MCP/A2A surface
      --operator-session string   scope audit to a specific Operator session
      --insecure                 skip TLS verify (local dev only)
      --out string               report output dir
      --l3-mode string           mock|suspend
      --ensemble int             mock consensus voters (default 3)
      --verbose                  echo each request/response
      --phase string             doctrine|notary|all (default "all")
  -h, --help                     help for run
```

### agent-harness audit
```
Audit signed receipts from the Operator

Usage:
  g8e agent-harness audit [flags]

Flags:
      --config string            JSON config overlay
      --mtls-url string          Gateway mTLS surface
      --public-url string        Gateway public surface
      --cert string              client cert PEM
      --key string               client key PEM
      --ca string                gateway CA bundle PEM
      --api-key string           operator API key
      --operator-session string   operator session id
      --insecure                 skip TLS verify
      --out string               report output dir
  -h, --help                     help for audit
```
