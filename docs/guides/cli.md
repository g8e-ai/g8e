# CLI Reference

This reference documents the g8e CLI commands for managing the Governance Gateway (g8eg) and Governed Operator (g8eo).

## g8e Root Help
```
g8e is a zero-trust execution platform for agentic infrastructure.
The CLI manages the Governance Gateway (g8eg) and Governed Operator (g8eo).

Usage:
  g8e [command]

Available Commands:
  approve     Approve a suspended L3 transaction with CLI signature
  auth        Authentication and session management
  auditor     Universal agent emulator for a real g8e Gateway/Operator
  chaos       Generate realistic governance events against the local g8e audit stack
  data        Administer the local platform over mTLS
  gw          Manage the Governance Gateway (g8eg) lifecycle
  help        Help about any command
  security    Security validation checks
  test        Run test suites

Flags:
  -h, --help   help for g8e

Use "g8e [command] --help" for more information about a command.
```


## gw
```
Gateway lifecycle commands for starting, stopping, and checking the status of the Governance Gateway.

Usage:
  g8e gw [command]

Aliases:
  gateway

Available Commands:
  clean           Destructively remove all Gateway state
  logs            View Gateway logs
  mcp-config      Print MCP client configuration for the Gateway
  mcp-config-http Print MCP client configuration for the Gateway plain HTTP endpoint
  reset           Reset Gateway data and secrets (preserves CA)
  restart         Restart the Governance Gateway
  settings        Manage Gateway settings
  start           Start the Governance Gateway
  status          Check Gateway health and status
  stop            Stop the Governance Gateway

Flags:
  -h, --help   help for gw

Use "g8e gw [command] --help" for more information about a command.
```

### gw start
```
Start the Governance Gateway

Usage:
  g8e gw start [flags]

Flags:
      --bootstrap-port int       Bootstrap TLS port for CSR enrollment (default: from paths.json)
      --cert-mode string         Certificate mode: full (all hostnames/IPs), localhost (only localhost)
      --data-dir string          Data directory for SQLite database (default: .g8e/data in working directory)
  -h, --help                     help for start
      --http-port int            HTTPS port for mTLS API (default: from paths.json)
      --log string               Log level: info, error, debug (default "info")
      --mcp-http-port int        Plain HTTP port for MCP calls (default: from paths.json)
      --passkey-rp-id string     RP ID for passkey operations (default: localhost)
      --passkey-rp-name string   RP Name for passkey operations (default: g8e)
      --pki-dir string           Directory for TLS certificates (default: .g8e/pki)
      --posture string           Gateway posture: doctrine (L1 enforced, L2/L3 audited), consensus (L1/L2 enforced, L3 audited), notary (L1/L2/L3 strictly enforced) (default "doctrine")
      --public-port int          Public browser/BYO bootstrap port (default: from paths.json)
      --rate-limit-burst int     Gateway rate limit burst size
      --rate-limit-rps float     Gateway requests per second limit (set to 0 to disable)
      --secrets-dir string       Directory for platform secrets (default: .g8e/secrets)
```

When `--cert-mode full` is selected, the CLI detects network identity once, writes it to a temporary JSON file in the runtime directory, and passes that file to the Gateway subprocess via `--network-identity-file`. `--cert-mode localhost` continues to use loopback-only identities, including IPv6 localhost when available.

### gw stop
```
Stop the Governance Gateway

Usage:
  g8e gw stop [flags]

Flags:
  -h, --help   help for stop
```

### gw status
```
Check Gateway health and status

Usage:
  g8e gw status [flags]

Flags:
  -h, --help   help for status
```

### gw restart
```
Restart the Governance Gateway

Usage:
  g8e gw restart [flags]

Flags:
  -h, --help   help for restart
```

### gw logs
```
View Gateway logs

Usage:
  g8e gw logs [flags]

Flags:
  -f, --follow   Follow log output (like tail -f)
  -h, --help     help for logs
```

### gw settings
```
Manage Gateway settings

Usage:
  g8e gw settings [flags]

Flags:
  -h, --help   help for settings
```

### gw reset
```
Reset Gateway data and secrets (preserves CA)

Usage:
  g8e gw reset [flags]

Flags:
      --force   Skip confirmation prompt
  -h, --help    help for reset
      --y       Skip confirmation prompt (shorthand)
      --yes     Skip confirmation prompt (shorthand)
```

### gw clean
```
Destructively remove all Gateway state

Usage:
  g8e gw clean [flags]

Flags:
      --force   Skip confirmation prompt
  -h, --help    help for clean
      --y       Skip confirmation prompt (shorthand)
      --yes     Skip confirmation prompt (shorthand)
```

### gw mcp-config
```
Print MCP client configuration for the Gateway. This command outputs a JSON configuration for MCP clients using the unified /mcp endpoint with the g8e.local internal hostname.

Usage:
  g8e gw mcp-config [flags]

Flags:
  -h, --help   help for mcp-config
```

### gw mcp-config-http
```
Print MCP client configuration for the Gateway plain HTTP endpoint. This command outputs a static JSON configuration for the plain HTTP MCP endpoint using explicit 127.0.0.1 (localhost may resolve to IPv6 ::1).

Usage:
  g8e gw mcp-config-http [flags]

Flags:
  -h, --help   help for mcp-config-http
```

## auth
```
Manage mTLS enrollment and operator sessions.

Usage:
  g8e auth [command]

Available Commands:
  login           Authenticate and save operator session
  logout          Clear local operator session and credentials
  enroll-windows  Enroll via Windows Certificate Store (Windows only)

Flags:
  -h, --help   help for auth

Use "g8e auth [command] --help" for more information about a command.
```

### auth login
```
Authenticate CLI with the running Gateway via CSR-based enrollment. Generates client keypairs, submits CSRs to the Gateway's CA, and saves signed mTLS credentials. The Gateway must already be running (use './g8e gw start' first).

Usage:
  g8e auth login [flags]

Flags:
  -h, --help   help for login
```

### auth logout
```
Clear local operator session and credentials

Usage:
  g8e auth logout [flags]

Flags:
  -h, --help   help for logout
```

### auth enroll-windows
```
Enroll via Windows Certificate Store (Windows only). Generate an ECDSA P-384 keypair in the Windows Certificate Store, submit a CSR to the Gateway, and import the signed certificate. Chrome/Edge will automatically present this cert.

Usage:
  g8e auth enroll-windows [flags]

Flags:
      --tpm   Use TPM-backed key via Windows Hello for Business
  -h, --help   help for enroll-windows
```

## data
```
Data management commands for users, operators, and settings.

Usage:
  g8e data [command]

Available Commands:
  audit        Query audit vault
  operators    Manage operator instances
  settings     Manage Gateway settings
  store        Manage document storage
  users        Manage user accounts

Flags:
  -h, --help   help for data

Use "g8e data [command] --help" for more information about a command.
```

### data users
```
Manage user accounts

Usage:
  g8e data users [flags]

Flags:
  -h, --help   help for users
```

### data operators
```
Manage operator instances

Usage:
  g8e data operators [flags]

Flags:
  -h, --help   help for operators
```

### data settings
```
Manage Gateway settings

Usage:
  g8e data settings [flags]

Flags:
  -h, --help   help for settings
```

### data store
```
Manage document storage

Usage:
  g8e data store [flags]

Flags:
      --collection string    Collection name
      --document-id string   Document ID (omit to list collection)
  -h, --help                 help for store
```

### data audit
```
Query audit vault

Usage:
  g8e data audit [command]

Available Commands:
  list        List audit events for a session
  summary     Show chaos test summary from audit vault

Flags:
  -h, --help   help for audit

Use "g8e data audit [command] --help" for more information about a command.
```

#### data audit list
```
List audit events for a session

Usage:
  g8e data audit list [flags]

Flags:
  -h, --help                         help for list
      --limit int                    Limit number of events (default 100)
      --operator-session-id string   Operator session ID
```

#### data audit summary
```
Show chaos test summary from audit vault

Usage:
  g8e data audit summary [flags]

Flags:
  -h, --help   help for summary
```

## test
```
Run test suites. Use 'test ci' to mirror GitHub Actions CI exactly.

Usage:
  g8e test [flags]
  g8e test [command]

Available Commands:
  ci          Run full CI test suite (mirrors GitHub Actions exactly)
  integration Run integration tests
  review      Review integration test vault results
  scenario    Run scenario integration tests
  summary     Show summary of all integration test results
  unit        Run unit tests

Flags:
  -h, --help   help for test

Use "g8e test [command] --help" for more information about a command.
```

### test unit
```
Run unit tests

Usage:
  g8e test unit [flags]

Flags:
      --coverage     Generate coverage report
  -h, --help         help for unit
      --race         Enable race detector (default true)
      --run string   Run specific test (regex)
      --v            Verbose output
```

### test integration
```
Run integration tests

Usage:
  g8e test integration [flags]

Flags:
  -h, --help         help for integration
      --run string   Run specific scenario (e.g., forge_signature)
```


### test ci
```
Run the full CI pipeline locally: proto generation, linting, vulncheck, and platform tests with platform start/stop and coverage enforcement.

Usage:
  g8e test ci [flags]

Flags:
  -h, --help   help for ci
```


### test scenario
```
Run scenario integration tests

Usage:
  g8e test scenario [flags]

Flags:
  -h, --help         help for scenario
      --run string   Run specific scenario (e.g., forge_signature)
  -v, --verbose      Verbose output
```

### test summary
```
Show summary of all integration test results

Usage:
  g8e test summary [flags]

Flags:
  -h, --help   help for summary
```

## security
```
Run security validation and PKI verification checks.

Usage:
  g8e security [command]

Available Commands:
  pki         PKI management
  validate    Run security validation checks

Flags:
  -h, --help   help for security

Use "g8e security [command] --help" for more information about a command.
```

### security validate
```
Run security validation checks

Usage:
  g8e security validate [flags]

Flags:
  -h, --help                 help for validate
      --pki-dir string       PKI directory (default: .g8e/pki)
      --secrets-dir string   Secrets directory (default: .g8e/secrets)
```

### security pki
```
PKI management

Usage:
  g8e security pki [command]

Available Commands:
  enroll      Enroll a device with the Gateway via CSR

Flags:
  -h, --help   help for pki

Use "g8e security pki [command] --help" for more information about a command.
```

#### security pki enroll
```
Enroll a device with the Gateway via CSR. Generate a CSR and enroll with the Gateway to obtain mTLS certificates.

Usage:
  g8e security pki enroll [flags]

Flags:
  -e, --endpoint string     Gateway IP address (e.g., 192.168.1.62)
  -h, --help                help for enroll
      --output-dir string   Output directory for certificates (default: project root)
```

## approve
```
Approve a suspended L3 transaction with CLI signature. Approve a suspended transaction by signing the transaction hash with the CLI private key and submitting the cryptographic proof to the Gateway.

Usage:
  g8e approve <transaction_hash> [flags]

Flags:
  -h, --help   help for approve
```

## auditor
```
Universal agent emulator for a real g8e Gateway/Operator. auditor impersonates arbitrary AI tools and agents against a REAL g8e Gateway + Operator, exercising the full protocol surface (MCP, A2A, A2A protobuf, and official governance envelopes with mock consensus + principal signing), then audits every result against the Operator's signed receipts.

Usage:
  g8e auditor [command]

Available Commands:
  audit       Audit signed receipts from the Operator
  list        List available scenarios
  run         Run scenarios against a real Gateway/Operator

Flags:
  -h, --help   help for auditor

Use "g8e auditor [command] --help" for more information about a command.
```

### auditor list
```
List available scenarios

Usage:
  g8e auditor list [flags]

Flags:
  -h, --help   help for list
```

### auditor run
```
Run scenarios against a real Gateway/Operator

Usage:
  g8e auditor run [flags] [scenario ...]

Flags:
      --api-key string           operator API key for MCP/A2A surface
      --ca string                gateway CA bundle PEM
      --cert string              client cert PEM
      --config string            JSON config overlay
      --ensemble int             mock consensus voters (default 3)
      --insecure                 skip TLS verify (local dev only)
      --key string               client key PEM
      --l3-mode string           mock|suspend
      --mtls-url string          Gateway mTLS surface
      --out string               report output dir
      --operator-session string   scope audit to a specific operator session
      --phase string             doctrine|notary|all (default "all")
      --public-url string        Gateway public surface for OOB approve
  -h, --help                     help for run
      --verbose                  echo each request/response
```

### auditor audit
```
Audit signed receipts from the Operator

Usage:
  g8e auditor audit [flags]

Flags:
      --api-key string           operator API key
      --ca string                gateway CA bundle PEM
      --cert string              client cert PEM
      --config string            JSON config overlay
      --insecure                 skip TLS verify
      --key string               client key PEM
      --mtls-url string          Gateway mTLS surface
      --out string               report output dir
      --operator-session string   operator session id
      --public-url string        Gateway public surface
  -h, --help                     help for audit
```

## chaos
```
Generate realistic governance events against the local g8e audit stack. chaos generates a realistic distribution of governance events against the local g8e audit stack. It bypasses network/TLS by driving the TransactionVerifier + Actuator stack directly in-process, which is the same path exercised by the live operator when payloads arrive over pub/sub.

Distribution:
  70%  Good Actor  – valid sig, safe intent (FS_LIST)       → EXECUTED
  20%  Prompt Inj  – valid sig, L1 forbidden cmd (sudo/rm)  → REJECTED (L1)
  10%  MitM        – corrupted transaction hash              → REJECTED (hash mismatch)

Usage:
  g8e chaos [flags]

Flags:
      --count int      number of payloads to fire (default 100)
      --data-dir string audit vault data dir (default: <project-root>/.g8e/test-vault/<timestamp>)
  -h, --help            help for chaos
      --pki-dir string  PKI dir for trusted_signers (default: <cwd>/.g8e/pki)
