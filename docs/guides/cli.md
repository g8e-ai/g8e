# CLI Reference

This reference is auto-generated from the Cobra CLI help output.

## g8e Root Help
```
g8e is a zero-trust execution platform for agentic infrastructure.
The CLI manages the Governance Gateway (g8eg) and Governed Operator (g8eo).

Usage:
  g8e [command]

Available Commands:
  auth        Authentication and session management
  data        Administer the local platform over mTLS
  gw          Manage the Governance Gateway (g8eg) lifecycle
  help        Help about any command
  security    Security validation checks
  setup       Bootstrap platform dependencies and configuration
  test        Run test suites

Flags:
  -h, --help   help for g8e

Use "g8e [command] --help" for more information about a command.
```

## setup
```
Setup checks for required dependencies (Go, Python), generates protocol artifacts, and builds services.

Usage:
  g8e setup [flags]

Flags:
  -h, --help   help for setup
```

## gw
```
Gateway lifecycle commands for starting, stopping, and checking the status of the Governance Gateway.

Usage:
  g8e gw [command]

Aliases:
  gateway

Available Commands:
  clean       Destructively remove all Gateway state
  logs        View Gateway logs
  reset       Reset Gateway data and secrets (preserves CA)
  restart     Restart the Governance Gateway
  settings    Manage Gateway settings
  start       Start the Governance Gateway
  status      Check Gateway health and status
  stop        Stop the Governance Gateway

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
  -h, --help   help for start
```

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

## auth
```
Manage mTLS enrollment and operator sessions.

Usage:
  g8e auth [command]

Available Commands:
  login       Authenticate and save operator session
  logout      Clear local operator session and credentials

Flags:
  -h, --help   help for auth

Use "g8e auth [command] --help" for more information about a command.
```

### auth login
```
Authenticate and save mTLS credentials to ~/.g8e/credentials. The first login automatically bootstraps the platform.

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
  chaos       Run chaos engineering tests
  ci          Run full CI test suite (mirrors GitHub Actions exactly)
  g8eo        Run Gateway (g8eo) tests
  integration Run integration tests
  review      Review integration test vault results
  scenario    Run scenario integration tests
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

### test g8eo
```
Run Gateway (g8eo) tests

Usage:
  g8e test g8eo [flags]

Flags:
      --coverage     Generate coverage report
  -h, --help         help for g8eo
      --race         Enable race detector (default true)
      --run string   Run specific test (regex)
      --v            Verbose output
```

### test ci
```
Runs make ci which includes proto generation, linting, vulncheck, and platform tests with platform start/stop and coverage enforcement. This is the canonical way to replicate CI locally.

Usage:
  g8e test ci [flags]

Flags:
  -h, --help   help for ci
```

### test chaos
```
Run chaos engineering tests

Usage:
  g8e test chaos [flags]

Flags:
      --count int   Number of payloads to fire (default: 100)
  -h, --help        help for chaos
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

## security
```
Run security validation and PKI verification checks.

Usage:
  g8e security [command]

Available Commands:
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

