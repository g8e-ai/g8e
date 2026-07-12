# g8e Protocol

Protocol library for the g8e zero-trust execution platform. Provides protobuf schema definitions, JSON constant registries, JSON model schemas, Pydantic models, dynamic enum generation, SPIFFE workload identity helpers, and example programs for building g8e-compatible clients and services.

## Installation

### Go

The Go module requires Go 1.26 and depends on `google.golang.org/grpc` and `google.golang.org/protobuf`. Install with `go get github.com/g8e-ai/g8e/protocol`.

### Python

The Python package requires Python 3.10 or later and depends on `pydantic>=2.0.0` and `protobuf>=4.0.0`. Install with `pip install g8e`.

## Directory Structure

```
protocol/
  go.mod                     Go module definition (github.com/g8e-ai/g8e/protocol)
  go.sum                     Go dependency checksums
  go_package.go              Package doc for the Go protocol package
  workload_identity.go       SPIFFE workload identity generation and validation
  workload_identity_test.go  Unit tests for workload identity helpers
  Makefile                   Test, format, vet, lint, and OpenAPI targets
  vendor/                    Vendored Go dependencies
  proto/                     Protobuf schema definitions and generated Go code
    buf.yaml                 Buf module config (buf.build/g8e/platform)
    g8e/common/v1/           Common governance types (common.proto, common.pb.go)
    g8e/operator/v1/         Operator service types (operator.proto, operator.pb.go, operator_grpc.pb.go)
    g8e/pubsub/v1/           Pub/sub message types (pubsub.proto, pubsub.pb.go)
  constants/                 JSON protocol constant registries
    doctrine/                L1 Doctrine pattern registries
  models/                    JSON model schemas and Python error enums
    agents/                  Per-agent-role model schemas
    errors.py                Python error code and category enums
  python/                    Python package (g8e-protocol)
    g8e/                     Import namespace (g8e)
      constants.py           Runtime loader for JSON protocol constants
      enums.py               Dynamic StrEnum/IntEnum generation from STATUS and EVENTS
      models/                Pydantic v2 models for protocol data structures
    examples/                Python example scripts
    pyproject.toml           Package metadata and dependencies
    README.md                Python package README
  examples/                  Go example programs
    governance_envelope/     GovernanceEnvelope construction demo
    workload_identity/       SPIFFE workload identity demo
    mcp_server/              Example MCP server configuration
  docs/                      Protocol reference documentation
    spec.md                  Protocol specification (GovernanceEnvelope, 5-layer interlock)
    constants.md             Constants system reference
    a2a.md                   A2A protocol specification and integration
    a2a.proto                A2A upstream protocol protobuf definition (reference)
    a2a.json                 A2A JSON Schema bundle (non-normative)
    mcp.md                   MCP protocol specification and integration
    mcp.json                 MCP payload type JSON Schema bundle (non-normative)
    mcp_jsonrpc_schema.json  MCP JSON-RPC 2.0 message schema
    mcp_tool_template.go     Native tool implementation template
    reference/api/           Generated Markdown API documentation from protobuf
```

The buf generation config resides at `buf.gen.yaml` in the repository root. It produces Go structs and gRPC service stubs into `protocol/proto`, and Markdown API documentation into `protocol/docs/reference/api`.

## Components

### Protobuf Schemas

Schema definitions reside in `proto/` and are managed with [buf](https://buf.build). The buf configuration is in `proto/buf.yaml` (module: `buf.build/g8e/platform`). Code generation is configured in `buf.gen.yaml` at the repository root.

- **proto/g8e/common/v1** (`common.proto`): Core governance types. Defines `GovernanceEnvelope`, `GovernanceMetadata`, `L1Metadata`, `L2Metadata` (with `L2Vote` and `tribunal_id`), `L3Metadata` (with `L3Proof`), the `Component` enum, and the `forbidden_patterns` field option used by L1 Doctrine validation. The `GovernanceEnvelope` carries identity fields (`operator_id`, `operator_session_id`, `web_session_id`, `cli_session_id`, `requestor_user_id`, `acting_app_id`), intent fields (`event_type`, `payload`, `intent_data`, `action_type`, `target_resource`), state and replay protection fields (`state_merkle_root`, `nonce`, `transaction_hash`, `protocol_version`), governance proofs, and application context (`case_id`, `investigation_id`, `task_id`, `system_fingerprint`, `tenant_id`, `binding_persona`).
- **proto/g8e/operator/v1** (`operator.proto`): Operator service definitions. Defines the `OperatorService` gRPC service with RPCs for `ExecuteCommand`, `CancelCommand`, `EditFile`, `ListFileSystem`, and `ReadFileSystem`. Defines the `ExecutionStatus`, `L2Status`, `L3Status`, and `HeartbeatType` enums. Contains request payload messages for command execution, file editing, filesystem operations (list, read, grep), heartbeats, port checks, log fetching, audit history and file diff retrieval, file restoration, direct command audit, certificate signing and revocation, device link management, operator binding and termination, target context setting, shutdown, eval answer submission, MCP tool dispatch and resource/prompt operations, A2A skill dispatch, and passkey/WebAuthn registration, authentication, and credential management. Contains result messages including `CommandResult`, `FsListResult`, `FsReadResult`, `FsGrepResult`, `FileEditResult`, `PortCheckResult`, `FetchLogsResult`, `FetchHistoryResult`, `FetchFileHistoryResult`, `FetchFileDiffResult`, `RestoreFileResult`, and `HeartbeatResult` with telemetry sub-messages (`SystemIdentity`, `NetworkInfo`, `PerformanceMetrics`, `OSDetails`, `UserDetails`, `DiskDetails`, `MemoryDetails`, `EnvironmentDetails`, `FingerprintDetails`, `CapabilityFlags`, `VersionInfo`, `UptimeInfo`). Defines the `ActionReceipt` and `CommitmentAttestation` signed proof structures.
- **proto/g8e/pubsub/v1** (`pubsub.proto`): Pub/sub message types. Defines `PubSubMessage` and `PubSubEvent`.

Generated Go code exists alongside each `.proto` file as `.pb.go` and `_grpc.pb.go` files.

### Go Package

The root Go package (`protocol`) provides SPIFFE workload identity generation and validation for the `g8e.local` trust domain via `workload_identity.go`. It supports six identity types:

- **Operator**: `spiffe://g8e.local/operator/<org_id>/<operator_id>/<session_id>`
- **CLI**: `spiffe://g8e.local/cli/<user_id>/<session_id>`
- **App**: `spiffe://g8e.local/app/<operator_id>`
- **User**: `spiffe://g8e.local/user/<user_id>`
- **Hub**: `spiffe://g8e.local/hub/operator-listen`
- **GatewayPeer**: `spiffe://g8e.local/gateway/<gateway_id>`

Each identity type provides `SPIFFEID` and `SPIFFEURL` generation methods and `Matches*` validation methods. The `MatchesCLISessionOnly` method validates a CLI identity by session ID only, prior to loading user context. Extraction methods include `ExtractCLISessionID`, `ExtractUserID`, `ExtractUserIDFromUserSAN`, `ExtractOperatorSessionID`, and `ExtractGatewayID`.

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
- **doctrine/**: L1 Doctrine pattern registries (`doctrine_registry.json`, `gitleaks_doctrine.json`, `mcp_vectors_doctrine.json`, `owasp_crs_doctrine.json`, `blacklist_doctrine.json`, `whitelist_doctrine.json`)

### JSON Model Schemas

The `models/` directory contains JSON Schema files that define the structure for protocol data structures:

- `account_lock.json`, `app_policy.json`, `auth_admin_audit.json`, `bound_session.json`
- `case.json`, `cli_session.json`, `console_audit.json`, `conversation.json`, `conversation_message.json`
- `errors.py`: Python enums for error categories (`ErrorCategory`), severities (`ErrorSeverity`), command categories (`CommandCategory`), and error codes (`ErrorCode`)
- `investigation.json`, `login_audit.json`, `memory.json`
- `operator_document.json`, `operator_session.json`, `operator_usage.json`, `organization.json`
- `passkey_challenge.json`, `persona.json`, `platform_settings.json`
- `reputation_commitment.json`, `reputation_state.json`, `revoked_certificate.json`, `stake_resolution.json`
- `security_constraints.json`, `task.json`, `tool_results.json`, `tribunal.json`, `trusted_signer.json`
- `user.json`, `user_settings.json`, `web_session.json`
- `agent_activity_metadata.json`
- `agents/`: Per-agent-role model schemas (`primary.json`, `assistant.json`, `lite.json`, `triage.json`, `title_generator.json`, `agent_harness.json`)

### Python Package

The Python package is in `python/` and installs as `g8e`. The import namespace is `g8e`.

- **`g8e.constants`**: Runtime loader for JSON protocol constants. Loads all constant registries from `constants/` at import time. Exports `EVENTS`, `STATUS`, `MSG`, `COLLECTIONS`, `KV`, `CHANNELS`, `PUBSUB`, `INTENTS`, `PROMPTS`, `TIMESTAMP`, `HEADERS`, `DOCUMENT_IDS`, `PLATFORM`, `AGENTS`, `NETWORK`, `API_PATHS`, and the `ComponentName` StrEnum (`CLIENT`, `G8EE`, `G8EO`, `OPERATOR`). Also exports HTTP header name constants for session, context, and identity propagation.
- **`g8e.enums`**: Dynamic enum generation from protocol constants. Generates `StrEnum` and `IntEnum` classes from the `STATUS` and `EVENTS` dicts at access time using `__getattr__`. Integer-valued categories produce `IntEnum`; all others produce `StrEnum`. Enum member names are SCREAMING_SNAKE_CASE; values preserve the raw protocol wire format. Exports `EventType` and all status category enums.
- **`g8e.models`**: Pydantic v2 models for protocol data structures. Submodules:
  - `g8e.models.base`: `G8eBaseModel` base class with `UTCDatetime`, `Field`, `ConfigDict`, `ValidationError`, `field_validator`, `model_validator`
  - `g8e.models.context`: `RequestContext`, `BoundOperator`
  - `g8e.models.events`: SSE event wire models (`SessionEventWire`, `BackgroundEventWire`, `AiProcessingStoppedPayload`, `AIToolLifecyclePayload`, `ChatCitationsReadyPayload`, `ChatErrorPayload`, `ChatProcessingStartedPayload`, `ChatResponseChunkPayload`, `ChatResponseCompletePayload`, `ChatRetryPayload`, `ChatThinkingPayload`, `ChatTurnCompletePayload`, `TriageClarificationQuestionsPayload`)
  - `g8e.models.internal_api`: `ChatMessageRequest`, `ChatStartedResponse`, `ResourceCreationRequest`
  - `g8e.models.settings`: `PlatformSettings`, `G8eeUserSettings`, `LLMSettings`, `SearchSettings`, `EvalJudgeSettings`, `CommandValidationSettings`, `BatchExecutionSettings`

### Examples

The `examples/` directory contains Go example programs:

- `governance_envelope/main.go`: Demonstrates constructing a `GovernanceEnvelope` with L1, L2, and L3 governance metadata, a `CommandRequested` payload, and protojson round-trip serialization
- `workload_identity/main.go`: Demonstrates SPIFFE workload identity generation, validation, extraction, and URL parsing for all six identity types
- `mcp_server/g8e_gateway_mcp_config.json`: HTTP + mTLS MCP server configuration with literal cert paths for production deployments
- `mcp_server/g8e_gateway_mcp_config_env.json`: HTTP + mTLS MCP server configuration with env-var cert paths for containerized deployments
- `mcp_server/g8e_stdio_mcp_config.json`: Stdio MCP server configuration for local development with native tools and no gateway required

The `python/examples/` directory contains Python example scripts:

- `constants_example.py`: Demonstrates accessing event, status, and collection constants, component enums, and HTTP header construction
- `models_example.py`: Demonstrates `RequestContext` creation with bound operators, `PlatformSettings` configuration, and model validation

## Usage

See the Examples section above for Go and Python example programs. See [Protocol Specification](docs/spec.md) for governance envelope structure and the five-layer interlock sequence. See [Constants Reference](docs/constants.md) for the constants system. See [A2A Protocol](docs/a2a.md) and [MCP Protocol](docs/mcp.md) for protocol integration details.

## Development

The `Makefile` provides the following targets:

- `make test`: Run Go tests with race detection
- `make fmt`: Format Go code with `gofmt -s`
- `make vet`: Run `go vet`
- `make lint`: Run `golangci-lint`
- `make openapi`: Generate OpenAPI specification from protobuf (requires buf)

Protobuf code generation uses buf. The generation config is in `buf.gen.yaml` at the repository root. Run `buf generate` from the repository root to regenerate Go structs, gRPC stubs, and Markdown API documentation into `proto/`, `proto/`, and `docs/reference/api/` respectively.

## Protocol Versioning

This package follows semantic versioning. Major version changes indicate breaking protocol changes. Minor version changes add new protocol features. Patch version changes include bug fixes and non-breaking enhancements.

## License

Apache License 2.0. See `LICENSE` for details.

## Contributing

Protocol changes require coordination across all g8e components. Submit protocol change proposals via GitHub issues with clear justification and impact analysis.

## Support

For protocol questions and support, open a GitHub issue or visit https://github.com/g8e-ai/g8e
