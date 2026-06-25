# g8e Protocol

Protocol library for the g8e zero-trust execution platform. Provides protobuf schema definitions, JSON constant registries, JSON model schemas, Pydantic models, SPIFFE workload identity helpers, and test fixtures for building g8e-compatible clients and services.

## Installation

### Go

```bash
go get github.com/g8e-ai/g8e/protocol
```

The Go module requires Go 1.26 and depends on `google.golang.org/grpc` and `google.golang.org/protobuf`.

### Python

```bash
pip install g8e-protocol
```

The Python package requires Python 3.10 or later and depends on `pydantic>=2.0.0` and `protobuf>=4.0.0`.

## Directory Structure

```
protocol/
  go.mod                  Go module definition (github.com/g8e-ai/g8e/protocol)
  go_package.go           Package doc for the Go protocol package
  workload_identity.go    SPIFFE workload identity generation and validation
  workload_identity_test.go  Unit tests for workload identity helpers
  Makefile                Test, format, vet, lint, and OpenAPI targets
  buf.gen.yaml            Buf generation config (at repository root)
  proto/                  Protobuf schema definitions and generated Go code
  constants/              JSON protocol constant registries
  models/                 JSON model schemas and Python Pydantic models
  python/                 Python package (g8e-protocol)
  examples/               Go example programs
  docs/                   Protocol reference documentation (placeholder)
```

## Components

### Protobuf Schemas

Schema definitions reside in `proto/` and are managed with [buf](https://buf.build). The buf configuration is in `proto/buf.yaml` (module: `buf.build/g8e/platform`). Code generation is configured in `buf.gen.yaml` at the repository root, which produces Go structs, gRPC service stubs, and Markdown API documentation.

- **proto/g8e/common/v1** (`common.proto`): Core governance types. Defines `GovernanceEnvelope`, `GovernanceMetadata`, `L1Metadata`, `L2Metadata` (with `L2Vote`), `L3Metadata` (with `L3Proof`), the `Component` enum, and the `forbidden_patterns` field option used by L1 Doctrine validation.
- **proto/g8e/operator/v1** (`operator.proto`): Operator service definitions. Defines the `OperatorService` gRPC service with RPCs for `ExecuteCommand`, `CancelCommand`, `EditFile`, `ListFileSystem`, and `ReadFileSystem`. Contains payload messages for command execution, file operations, filesystem queries, port checks, log fetching, audit history, certificate signing and revocation, device link management, operator binding, MCP and A2A tool dispatch, passkey/WebAuthn registration and authentication, heartbeats, and the `ActionReceipt` and `CommitmentAttestation` signed proof structures.
- **proto/g8e/pubsub/v1** (`pubsub.proto`): Pub/sub message types. Defines `PubSubMessage` and `PubSubEvent`.

Generated Go code exists alongside each `.proto` file as `.pb.go` and `_grpc.pb.go` files.

### Go Package

The root Go package (`protocol`) provides:

- **`workload_identity.go`**: SPIFFE workload identity generation and validation for the `g8e.local` trust domain. Supports six identity types:
  - **Operator**: `spiffe://g8e.local/operator/<org_id>/<operator_id>/<session_id>`
  - **CLI**: `spiffe://g8e.local/cli/<user_id>/<session_id>`
  - **App**: `spiffe://g8e.local/app/<operator_id>`
  - **User**: `spiffe://g8e.local/user/<user_id>`
  - **Hub**: `spiffe://g8e.local/hub/operator-listen`
  - **GatewayPeer**: `spiffe://g8e.local/gateway/<gateway_id>`

  Each identity type provides `SPIFFEID` and `SPIFFEURL` generation methods, `Matches*` validation methods, and `Extract*` methods for parsing SPIFFE IDs. The `MatchesCLISessionOnly` method validates a CLI identity by session ID only, prior to loading user context.

### Constants Registries

The `constants/` directory contains JSON files that serve as the authoritative source for protocol identifiers, paths, and configuration values across all g8e components:

- **events.json**: Event type identifiers for all governance, operator, and platform events
- **status.json**: Status code definitions for operator sessions, executions, and components
- **collections.json**: Database collection names
- **api_paths.json**: API endpoint paths
- **auth.json**: Authentication-related constants
- **headers.json**: HTTP header names for session, context, and identity propagation
- **kv_keys.json**: Key-value store key names
- **channels.json**: Pub/sub channel names
- **pubsub.json**: Pub/sub event type definitions
- **intents.json**: Intent classification identifiers
- **prompts.json**: Prompt template identifiers
- **agents.json**: Agent role definitions
- **platform.json**: Platform-wide configuration constants
- **senders.json**: Message sender identifiers
- **exit_codes.json**: Process exit code definitions
- **field_paths.json**: Structured field path identifiers
- **document_ids.json**: Document type identifiers
- **network.json**: Network-related constants
- **output.json**: Output format constants
- **ports.json**: Default port assignments
- **timestamp.json**: Timestamp format constants
- **env_vars.json**: Environment variable names
- **doctrine/**: L1 Doctrine pattern registries (`doctrine_registry.json`, `gitleaks_doctrine.json`, `mcp_vectors_doctrine.json`, `owasp_crs_doctrine.json`)

### JSON Model Schemas

The `models/` directory contains JSON Schema files that define the structure for protocol data structures:

- `case.json`, `conversation.json`, `conversation_message.json`, `investigation.json`
- `operator_document.json`, `user.json`, `user_settings.json`, `platform_settings.json`
- `tool_results.json`, `agent_activity_metadata.json`
- `reputation_commitment.json`, `reputation_state.json`, `stake_resolution.json`
- `security_constraints.json`
- `agents/`: Per-agent-role model schemas (`primary.json`, `assistant.json`, `lite.json`, `triage.json`, `title_generator.json`, `agent_harness.json`)

### Python Package

The Python package is in `python/` and installs as `g8e-protocol`. The import namespace is `g8e`.

- **`g8e.constants`**: Runtime loader for JSON protocol constants. Loads all constant registries from `constants/` at import time. Exports `EVENTS`, `STATUS`, `COLLECTIONS`, `KV`, `CHANNELS`, `PUBSUB`, `INTENTS`, `PROMPTS`, `HEADERS`, `DOCUMENT_IDS`, `PLATFORM`, `AGENTS`, `NETWORK`, `MSG`, `TIMESTAMP`, and the `ComponentName` StrEnum (`CLIENT`, `G8EE`, `G8EO`, `OPERATOR`). Also exports HTTP header name constants for session, context, and identity propagation.
- **`g8e.models`**: Pydantic v2 models for protocol data structures. Submodules:
  - `g8e.models.base`: `G8eBaseModel` base class with `UTCDatetime`, `Field`, `ConfigDict`
  - `g8e.models.context`: `RequestContext`, `BoundOperator`
  - `g8e.models.events`: SSE event wire models (`SessionEventWire`, `BackgroundEventWire`, chat payload models, tool lifecycle payload, triage clarification payload)
  - `g8e.models.internal_api`: `ChatMessageRequest`, `ChatStartedResponse`, `ResourceCreationRequest`
  - `g8e.models.settings`: `PlatformSettings`, `G8eeUserSettings`, `LLMSettings`, `SearchSettings`, `EvalJudgeSettings`, `CommandValidationSettings`, `BatchExecutionSettings`

### Examples

The `examples/` directory contains Go example programs:

- `governance_envelope/main.go`: Demonstrates constructing a `GovernanceEnvelope` with L1 metadata and a `CommandRequested` payload
- `workload_identity/main.go`: Demonstrates SPIFFE workload identity generation and validation
- `mcp_server/g8e_gateway_mcp_config.json`: Example MCP server configuration for the g8e gateway

## Usage

### Go - Workload Identity

```go
package main

import (
    "fmt"
    "github.com/g8e-ai/g8e/protocol"
)

func main() {
    wid := protocol.NewWorkloadIdentity()

    // Generate Operator SPIFFE ID
    spiffeID := wid.OperatorSPIFFEID("org-123", "op-456", "session-789")
    fmt.Println(spiffeID) // spiffe://g8e.local/operator/org-123/op-456/session-789

    // Validate SPIFFE ID
    if wid.MatchesOperator(spiffeID, "org-123", "op-456", "session-789") {
        fmt.Println("Valid Operator identity")
    }

    // Generate CLI SPIFFE ID
    cliID := wid.CLISPIFFEID("user-123", "cli-session-456")
    fmt.Println(cliID) // spiffe://g8e.local/cli/user-123/cli-session-456

    // Extract CLI session ID from SPIFFE ID
    sessionID, ok := wid.ExtractCLISessionID(cliID)
    if ok {
        fmt.Println(sessionID) // cli-session-456
    }
}
```

### Go - Governance Envelope

```go
package main

import (
    "github.com/g8e-ai/g8e/protocol/proto/g8e/common/v1"
    "github.com/g8e-ai/g8e/protocol/proto/g8e/operator/v1"
)

func main() {
    envelope := &commonv1.GovernanceEnvelope{
        Id: "txn-123",
        EventType: "g8e.v1.operator.command.requested",
        OperatorId: "op-456",
        OperatorSessionId: "session-789",
        Governance: &commonv1.GovernanceMetadata{
            L1: &commonv1.L1Metadata{
                Validated: true,
            },
        },
    }

    cmd := &operatorv1.CommandRequested{
        Command: "ls -la",
        ExecutionId: "exec-123",
        Justification: "List directory contents",
    }
}
```

### Python - Constants

```python
from g8e.constants import EVENTS, STATUS, COLLECTIONS, ComponentName

# Access protocol constants
print(EVENTS["command"]["requested"])
print(COLLECTIONS["operators"])
print(ComponentName.G8EO)
```

### Python - Models

```python
from g8e.constants import ComponentName
from g8e.models import RequestContext, BoundOperator

context = RequestContext(
    web_session_id="web-123",
    user_id="user-456",
    source_component=ComponentName.CLIENT,
    bound_operators=[
        BoundOperator(operator_id="op-789", operator_session_id="session-abc")
    ]
)
```

### Python - Headers

```python
from g8e.constants import (
    HTTP_AUTHORIZATION_HEADER,
    WEB_SESSION_ID_HEADER,
    CLI_SESSION_ID_HEADER,
    OPERATOR_ID_HEADER,
)

headers = {
    HTTP_AUTHORIZATION_HEADER: "Bearer token",
    WEB_SESSION_ID_HEADER: "web-123",
    CLI_SESSION_ID_HEADER: "cli-456",
    OPERATOR_ID_HEADER: "op-789",
}
```

## Development

The `Makefile` provides the following targets:

- `make test`: Run Go tests with race detection
- `make fmt`: Format Go code with `gofmt -s`
- `make vet`: Run `go vet`
- `make lint`: Run `golangci-lint`
- `make openapi`: Generate OpenAPI specification from protobuf (requires buf)

Protobuf code generation uses buf. The generation config is in `buf.gen.yaml` at the repository root. Run `buf generate` from the repository root to regenerate Go structs, gRPC stubs, and Markdown API documentation.

## Protocol Versioning

This package follows semantic versioning. Major version changes indicate breaking protocol changes. Minor version changes add new protocol features. Patch version changes include bug fixes and non-breaking enhancements.

## License

Apache License 2.0. See `LICENSE` for details.

## Contributing

Protocol changes require coordination across all g8e components. Submit protocol change proposals via GitHub issues with clear justification and impact analysis.

## Support

For protocol questions and support, open a GitHub issue or visit https://github.com/g8e-ai/g8e
