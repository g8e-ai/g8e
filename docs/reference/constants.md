# Constants System

## Single Source of Truth

The g8e constants system maintains a Single Source of Truth (SSOT) in JSON at `protocol/constants/`. All cross-component constants (collections, events, channels, headers, etc.) are defined as JSON files and generated into Go constants via `internal/constants/generate_registry.go`.

## Generation Flow

```text
JSON SSOT (protocol/constants/*.json)
    ↓
Go Registry (internal/constants/registry.go, status_generated.go, headers_generated.go) via generate_registry.go
```

### Step 1: JSON Source Files

Constants are defined in JSON files within `protocol/constants/`. Each file contains constant values with Go constant naming metadata:

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

### Step 2: Registry Generation

The `generate_registry.go` script reads JSON files from `protocol/constants/` and generates Go registry files (`registry.go`, `status_generated.go`, `headers_generated.go`) that provide runtime access to the constants snapshot.

## Tracked vs Internal Files

The constants system distinguishes between files that generate Go constants and internal-only files:

### Files That Generate Go Constants

The following JSON files are processed by `generate_registry.go`:

- `collections.json` - Database collection names
- `events.json` - Event type identifiers
- `headers.json` - HTTP header names
- `channels.json` - Pub/sub channel names
- `intents.json` - Intent classification values
- `document_ids.json` - Document ID prefixes
- `kv_keys.json` - Key-value store key patterns
- `senders.json` - Message sender identifiers
- `prompts.json` - Prompt template identifiers
- `pubsub.json` - Pub/sub protocol constants
- `status.json` - Internal enums (UserRole, OperatorStatus, ExecutionStatus)
- `platform.json` - Platform-specific constants
- `agents.json` - Agent persona details
- `timestamp.json` - Go-specific format strings

### Internal-Only Files (Not Processed by generate_registry.go)

The following files contain Go-specific constants not defined in JSON:

- `paths.go` - Filesystem paths
- `ports.go` - Network port numbers
- `api_paths.go` - API route paths
- `exit_codes.go` - Process exit codes
- `network.go` - Network-related constants
- `output.go` - Output format constants
- `mappings.go` - Mapping structures
- `auth.go` - Authentication-related constants

### Registry Validation Tracked Files

The `check_registry.go` script validates that constants in the following generated files are registered in `registry.go`:

- `headers_generated.go` (generated from headers.json)
- `collections.go`
- `events.go`
- `channels.go`
- `intents.go`
- `document_ids.go`
- `senders.go`
- `prompts.go`
- `pubsub.go`

Note that `kv_keys.json`, `status.json`, `platform.json`, `agents.json`, and `timestamp.json` are excluded from registry validation as they contain internal schemas or data not required in the runtime registry snapshot.

## Regeneration Commands

### Generate All Constants

```bash
make constants
```

This command runs `go run ./internal/constants/generate_registry.go` to generate Go registry files from JSON.

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

1. **Add the constant** to the appropriate JSON file in `protocol/constants/`
2. **Run `make constants`** to regenerate Go registry files
3. **Run `check_registry.go`** to verify the constant is registered
4. **Commit both** the JSON source file and the generated files

## CI Integration

The registry check is enforced in CI via the `registry-check` job in `.github/workflows/build-and-test.yml`. This ensures that any new constant added to a tracked file is properly registered before merging.

## Generated Files

The constants pipeline generates the following artifacts:

- Go registry files in `internal/constants/` - Runtime access to constants snapshot (registry.go, status_generated.go, headers_generated.go)

These generated files ensure consistency across the platform's Go components.

