# Constants System

## Single Source of Truth

The g8e constants system maintains a Single Source of Truth (SSOT) in Go at `internal/constants/`. All cross-component constants (collections, events, channels, headers, etc.) are defined as Go constants and exported to JSON via a generation pipeline.

## Export Flow

```text
Go SSOT (internal/constants/*.go)
    ↓
JSON Export (protocol/constants/*.json) via cmd/exporter
    ↓
Go Registry (internal/constants/registry.go, status_generated.go, headers_generated.go) via generate_registry.go
```

### Step 1: Go Source Files

Constants are defined in Go files within `internal/constants/`. Each file contains typed constants with naming metadata:

```go
const (
    CollectionUsers = "users"
    CollectionCases = "cases"
)
```

### Step 2: JSON Export

The `cmd/exporter` binary reads Go constants from the `internal/constants` package and exports them to JSON in `protocol/constants/`. Each JSON file includes:

- `value`: The constant value
- `_go_const`: The Go constant name

Example (`protocol/constants/collections.json`):
```json
{
  "collections": {
    "users": {
      "value": "users",
      "_go_const": "CollectionUsers"
    }
  }
}
```

The exporter supports merging hand-authored JSON extensions (e.g., documentation fields) with Go-generated constants, with Go SSOT taking precedence on conflicts.

### Step 3: Registry Generation

The `generate_registry.go` script reads JSON files from `protocol/constants/` and generates Go registry files (`registry.go`, `status_generated.go`, `headers_generated.go`) that provide runtime access to the constants snapshot.

## Tracked vs Internal Files

The constants system distinguishes between files exported to JSON and internal-only files:

### Files Exported to JSON (via cmd/exporter)

The following Go constant files are exported to `protocol/constants/*.json`:

- `collections.go` - Database collection names
- `events.go` - Event type identifiers
- `headers.go` - HTTP header names
- `channels.go` - Pub/sub channel names
- `intents.go` - Intent classification values
- `document_ids.go` - Document ID prefixes
- `kv_keys.go` - Key-value store key patterns
- `senders.go` - Message sender identifiers
- `prompts.go` - Prompt template identifiers
- `pubsub.go` - Pub/sub protocol constants
- `status.go` - Internal enums (UserRole, OperatorStatus, ExecutionStatus)
- `platform.go` - Platform-specific constants
- `agents.go` - Agent persona details
- `timestamp.go` - Go-specific format strings

### Internal-Only Files (Not Exported to JSON)

The following files contain Go-specific constants not exported to JSON:

- `paths.go` - Filesystem paths
- `ports.go` - Network port numbers
- `env_vars.go` - Environment variable names
- `api_paths.go` - API route paths
- `exit_codes.go` - Process exit codes
- `network.go` - Network-related constants
- `output.go` - Output format constants
- `mappings.go` - Mapping structures
- `auth.go` - Authentication-related constants

### Registry Validation Tracked Files

The `check_registry.go` script validates that constants in the following files are registered in `registry.go`:

- `collections.go`
- `events.go`
- `headers_generated.go` (generated from headers.json)
- `channels.go`
- `intents.go`
- `document_ids.go`
- `senders.go`
- `prompts.go`
- `pubsub.go`

Note that `kv_keys.go`, `status.go`, `platform.go`, `agents.go`, and `timestamp.go` are excluded from registry validation as they contain internal schemas or data not required in the runtime registry snapshot.

## Regeneration Commands

### Generate All Constants

```bash
make constants
```

This command:
1. Runs `go run ./internal/constants/generate_registry.go` to generate Go registry files from JSON
2. Builds the `g8e.exporter` binary from `cmd/exporter`
3. Runs the exporter to generate JSON files in `protocol/constants/` from Go SSOT

### Generate All Protocol Artifacts

```bash
make generate
```

Generates both protobuf code and constants.

### Clean Generated Constants

```bash
make clean-constants
```

Removes generated Go registry files (registry.go, status_generated.go, headers_generated.go).

## Registry Validation

The `check_registry.go` script validates that all tracked Go constants are registered in `registry.go`. This prevents drift between the Go source and the generated registry.

### Run Validation

```bash
go run ./internal/constants/check_registry.go
```

The script:
1. Parses tracked Go constant files (collections.go, events.go, headers_generated.go, channels.go, intents.go, document_ids.go, senders.go, prompts.go, pubsub.go)
2. Parses `registry.go` to extract registered constants
3. Reports any missing constants
4. Exits with code 1 if drift is detected

### Tracked Files Configuration

The `trackedFiles` map in `check_registry.go` defines which files should be validated. Internal-only files (kv_keys.go, status.go, platform.go, agents.go, timestamp.go) are excluded from registry validation as they contain internal schemas or data not required in the runtime registry snapshot.

## Adding New Constants

1. **Add the constant** to the appropriate Go file in `internal/constants/`
2. **Run `make constants`** to regenerate JSON exports and Go registry files
3. **Run `check_registry.go`** to verify the constant is registered
4. **Commit both** the Go source file and the generated files

## CI Integration

The registry check is enforced in CI via the `registry-check` job in `.github/workflows/build-and-test.yml`. This ensures that any new constant added to a tracked file is properly registered before merging.

## Generated Files

The constants pipeline generates the following artifacts:

- JSON files in `protocol/constants/` - Canonical constant definitions
- Go registry files in `internal/constants/` - Runtime access to constants snapshot (registry.go, status_generated.go, headers_generated.go)

These generated files ensure consistency across the platform's Go components.
