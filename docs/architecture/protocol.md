---
title: g8e Protocol Library
---

# g8e Protocol Library

Last Updated: 2026-07-24
Version: v1.6.2

The g8e Protocol Library is the canonical wire contract for all mutations in the g8e zero-trust execution platform. It provides protobuf schema definitions, JSON constant registries, JSON model schemas, Pydantic models, dynamic enum generation, SPIFFE workload identity helpers, and example programs for building g8e-compatible clients and services.

The protocol is published as two independent packages: a Go module (part of the root module) and a Python package, both sharing a single version number with the platform binary. There are no separate protocol-only releases; every g8e release ships the platform binary, the Go module, and the Python package at the same version.

---

## Table of Contents

- [Go Protocol Package](#go-protocol-package)
  - [Requirements](#go-requirements)
  - [Installation](#go-installation)
  - [Module Path & Versioning](#go-module-path--versioning)
  - [Package Contents](#go-package-contents)
  - [Protobuf Schemas](#go-protobuf-schemas)
  - [Workload Identity Helpers](#go-workload-identity-helpers)
  - [Usage Examples](#go-usage-examples)
  - [Development](#go-development)
- [Python Protocol Package](#python-protocol-package)
  - [Requirements](#python-requirements)
  - [Installation](#python-installation)
  - [Package Contents](#python-package-contents)
  - [Constants Module](#python-constants-module)
  - [Enums Module](#python-enums-module)
  - [Models Module](#python-models-module)
  - [Usage Examples](#python-usage-examples)
  - [Environment Configuration](#python-environment-configuration)
- [Shared Protocol Assets](#shared-protocol-assets)
  - [Constants Registries](#constants-registries)
  - [JSON Model Schemas](#json-model-schemas)
  - [MCP Server Example Configurations](#mcp-server-example-configurations)
- [Protobuf Code Generation](#protobuf-code-generation)
- [Release & Distribution](#release--distribution)
  - [Unified Versioning](#unified-versioning)
  - [Release Workflow](#release-workflow)
  - [CI Workflows](#ci-workflows)
  - [Version Sync Enforcement](#version-sync-enforcement)
- [Conformance Tests](#conformance-tests)
- [Directory Structure](#directory-structure)
- [References](#references)

---

## Go Protocol Package

### Go Requirements

- **Go 1.26** or later
- Dependencies: `google.golang.org/grpc v1.81.1`, `google.golang.org/protobuf v1.36.11`
- Transitive dependencies are vendored in the root `vendor/` directory

### Go Installation

```bash
go get github.com/g8e-ai/g8e@vX.Y.Z
```

To get the latest version:

```bash
go get github.com/g8e-ai/g8e@latest
```

### Go Module Path & Versioning

The Go protocol code is part of the root module `github.com/g8e-ai/g8e`. There is no separate `go.mod` for the protocol directory. The module version is derived from git tags of the form `vX.Y.Z`, which are created by `make release`. The Go module proxy resolves versions from these tags. See [Release & Distribution](#release--distribution) for details.

Import paths remain `github.com/g8e-ai/g8e/protocol/...`; the directory-relative import paths are unchanged by the module merge. Only the `require` line in a consumer's `go.mod` changes from `github.com/g8e-ai/g8e/protocol vX.Y.Z` to `github.com/g8e-ai/g8e vX.Y.Z`.

### Go Package Contents

The root Go package (`protocol`) provides:

- **`go_package.go`**: Package doc declaring this as the canonical protocol library
- **`workload_identity.go`**: SPIFFE workload identity generation and validation for the `g8e.local` trust domain
- **`workload_identity_test.go`**: Unit tests for workload identity helpers
- **`proto/`**: Generated Go code from protobuf schema definitions (`.pb.go` and `_grpc.pb.go` files)

### Go Protobuf Schemas

Schema definitions reside in `proto/` and are managed with [buf](https://buf.build). The buf configuration is in `proto/buf.yaml` (module: `buf.build/g8e/platform`). Code generation is configured in `buf.gen.yaml` at the repository root.

- **`proto/g8e/common/v1`** (`common.proto`): Core governance types. Defines `GovernanceEnvelope`, `GovernanceMetadata`, `L1Metadata`, `L2Metadata` (with `L2Vote` and `consensus_set_id`), `L3Metadata` (with `L3Proof`), the `Component` enum, and the `forbidden_patterns` field option used by L1 Doctrine validation. The `GovernanceEnvelope` carries identity fields (`operator_id`, `operator_session_id`, `web_session_id`, `cli_session_id`, `requestor_user_id`, `acting_app_id`), intent fields (`event_type`, `payload`, `intent_data`, `action_type`, `target_resource`), state and replay protection fields (`state_merkle_root`, `nonce`, `transaction_hash`, `protocol_version`), governance proofs, and application context (`case_id`, `investigation_id`, `task_id`, `system_fingerprint`, `tenant_id`, `binding_persona`).
- **`proto/g8e/operator/v1`** (`operator.proto`): Operator service definitions. Defines the `OperatorService` gRPC service with RPCs for `ExecuteCommand`, `CancelCommand`, `EditFile`, `ListFileSystem`, and `ReadFileSystem`. Defines the `ExecutionStatus`, `L2Status`, `L3Status`, and `HeartbeatType` enums. Contains request payload messages for command execution, file editing, filesystem operations, heartbeats, port checks, log fetching, audit history and file diff retrieval, file restoration, direct command audit, certificate signing and revocation, device link management, operator binding and termination, target context setting, shutdown, eval answer submission, MCP tool dispatch and resource/prompt operations, A2A skill dispatch, and passkey/WebAuthn registration, authentication, and credential management. Contains result messages including `CommandResult`, `FsListResult`, `FsReadResult`, `FsGrepResult`, `FileEditResult`, `PortCheckResult`, `FetchLogsResult`, `FetchHistoryResult`, `FetchFileHistoryResult`, `FetchFileDiffResult`, `RestoreFileResult`, and `HeartbeatResult` with telemetry sub-messages. Defines the `ActionReceipt` and `CommitmentAttestation` signed proof structures.
- **`proto/g8e/pubsub/v1`** (`pubsub.proto`): Pub/sub message types. Defines `PubSubMessage` and `PubSubEvent`.

Generated Go code exists alongside each `.proto` file as `.pb.go` and `_grpc.pb.go` files.

### Go Workload Identity Helpers

The `workload_identity.go` file provides SPIFFE workload identity generation and validation for the `g8e.local` trust domain. It supports six identity types:

- **Operator**: `spiffe://g8e.local/operator/<org_id>/<operator_id>/<session_id>`
- **CLI**: `spiffe://g8e.local/cli/<user_id>/<session_id>`
- **App**: `spiffe://g8e.local/app/<operator_id>`
- **User**: `spiffe://g8e.local/user/<user_id>`
- **Hub**: `spiffe://g8e.local/hub/operator-listen`
- **GatewayPeer**: `spiffe://g8e.local/gateway/<gateway_id>`

Each identity type provides `SPIFFEID` and `SPIFFEURL` generation methods. Validation methods include `MatchesOperator`, `MatchesCLI`, `MatchesCLISessionOnly`, `MatchesApp`, `MatchesHub`, and `MatchesGatewayPeer`. The `MatchesCLISessionOnly` method validates a CLI identity by session ID only, prior to loading user context. Extraction methods include `ExtractCLISessionID`, `ExtractUserID`, `ExtractUserIDFromUserSAN`, `ExtractOperatorSessionID`, and `ExtractGatewayID`.

### Go Usage Examples

Example programs are in `protocol/examples/`:

#### Governance Envelope Construction

`protocol/examples/governance_envelope/main.go` demonstrates constructing a `GovernanceEnvelope` with L1, L2, and L3 governance metadata, a `CommandRequested` payload, and protojson round-trip serialization. The example imports `commonv1` and `operatorv1` from the generated proto packages, populates identity and intent fields on the envelope, attaches a `CommandRequested` payload, and serializes/deserializes the result.

Run it from `protocol/examples/governance_envelope` with `go run main.go`.

#### Workload Identity

`protocol/examples/workload_identity/main.go` demonstrates SPIFFE workload identity generation, validation, extraction, and URL parsing for all six identity types. The example creates a `WorkloadIdentity` helper, generates SPIFFE IDs for each type, validates them with the corresponding `Matches*` methods, and extracts component IDs using the `Extract*` methods.

Run it from `protocol/examples/workload_identity` with `go run main.go`.

### Go Development

The `protocol/Makefile` provides the following targets:

| Target | Description |
|--------|-------------|
| `make test` | Run Go tests with race detection (`go test -race -count=1 ./...`) |
| `make fmt` | Format Go code with `gofmt -s` |
| `make vet` | Run `go vet` |
| `make lint` | Run `golangci-lint` |
| `make openapi` | Generate OpenAPI specification from protobuf (requires buf) |

---

## Python Protocol Package

### Python Requirements

- **Python 3.10** or later (tested on 3.10 through 3.14)
- Dependencies: `pydantic>=2.0.0`, `protobuf>=4.0.0`
- Build system: `setuptools>=61.0`, `wheel`

### Python Installation

From PyPI:

```bash
pip install g8e
```

Pinned to a specific version:

```bash
pip install g8e==X.Y.Z
```

### Python Package Contents

The Python package is in `protocol/python/` and installs as `g8e`. The import namespace is `g8e`. The package includes a `py.typed` marker for PEP 561 type-checker support. Unit tests are in `protocol/python/tests/` and cover constants loading, enum generation, model validation, and version sync.

- **`g8e/__init__.py`**: Package init exporting `__version__`
- **`g8e/constants.py`**: Runtime loader for JSON protocol constants from `protocol/constants/` or bundled `_data/` directory
- **`g8e/enums.py`**: Dynamic `StrEnum` and `IntEnum` generation from `STATUS`, `EVENTS`, `CHANNELS`, `INTENTS`, `PROMPTS`, `COLLECTIONS`, and `KV` protocol constants
- **`g8e/_data/`**: Bundled JSON protocol constants for PyPI installs (populated during release build)
- **`g8e/models/`**: Pydantic v2 models for protocol data structures
  - `base.py`: `G8eBaseModel` base class, `UTCDatetime` annotated type
  - `context.py`: `RequestContext`, `BoundOperator`
  - `events.py`: SSE event wire models and AI event payload models
  - `governance.py`: `GovernanceEnvelope`, `GovernanceMetadata`, `GovernanceL1`, `GovernanceL2`, `GovernanceL2Vote`, `GovernanceL3`, `GovernanceL3Proof`, and `compute_transaction_hash` utility
  - `internal_api.py`: `ChatMessageRequest`, `ChatStartedResponse`, `ResourceCreationRequest`, `LLMOverrides`
  - `settings.py`: `PlatformSettings`, `G8eeUserSettings`, and nested settings models

### Python Constants Module

The `g8e.constants` module loads JSON protocol constants from `protocol/constants/` at import time. It exports dicts for events, status, collections, headers, channels, pubsub, intents, prompts, timestamps, document IDs, platform configuration, agents, network, API paths, key-value keys, and sender identifiers.

Exported constants:

| Name | Source File |
|------|-------------|
| `EVENTS` | `events.json` |
| `STATUS` | `status.json` |
| `MSG` | `senders.json` |
| `COLLECTIONS` | `collections.json` |
| `KV` | `kv_keys.json` |
| `CHANNELS` | `channels.json` |
| `PUBSUB` | `pubsub.json` |
| `INTENTS` | `intents.json` |
| `PROMPTS` | `prompts.json` |
| `TIMESTAMP` | `timestamp.json` |
| `HEADERS` | `headers.json` |
| `DOCUMENT_IDS` | `document_ids.json` |
| `PLATFORM` | `platform.json` |
| `AGENTS` | `agents.json` |
| `NETWORK` | `network.json` |
| `API_PATHS` | `api_paths.json` |

The module also exports the `ComponentName` `StrEnum` (`CLIENT`, `G8EE`, `G8EO`, `OPERATOR`) and individual HTTP header string constants for session, context, and g8e-specific headers.

### Python Enums Module

The `g8e.enums` module dynamically generates `StrEnum` and `IntEnum` classes from the `STATUS`, `EVENTS`, `CHANNELS`, `INTENTS`, `PROMPTS`, `COLLECTIONS`, and `KV` protocol constants. Key characteristics:

- Enum member names use SCREAMING_SNAKE_CASE
- Values preserve the raw protocol wire format (e.g., `"g8e.v1.ai.llm.chat.iteration.started"`, `"user.cancelled"`, `"G8E-1000"`)
- Integer-valued categories produce `IntEnum`; all others produce `StrEnum`
- Enums are built lazily via `__getattr__` and cached with `lru_cache`
- Access enums by PascalCase name: `g8e.enums.OperatorToolName`, `g8e.enums.EventType`, `g8e.enums.Channel`, `g8e.enums.Intent`, `g8e.enums.Prompt`, `g8e.enums.Collection`, `g8e.enums.KVKey`

### Python Models Module

The `g8e.models` package provides Pydantic v2 models for protocol data structures. All models extend `G8eBaseModel`, which configures `populate_by_name` and `extra="ignore"`, and defaults `exclude_none` on serialization. The `UTCDatetime` annotated type serializes datetimes to ISO 8601 with a `Z` suffix.

| Submodule | Key Models |
|-----------|------------|
| `g8e.models.base` | `G8eBaseModel`, `UTCDatetime`, re-exports of `Field`, `ConfigDict`, `field_validator`, `model_validator`, `ValidationError` |
| `g8e.models.context` | `RequestContext`, `BoundOperator`: `RequestContext` validates session identity for `CLIENT` source components, requiring either `web_session_id` or `cli_session_id` and a `user_id` |
| `g8e.models.events` | `SessionEventWire`, `BackgroundEventWire`, and AI event payload models for chat processing, tool lifecycle, citations, errors, thinking, turn completion, retry, and triage clarification |
| `g8e.models.governance` | `GovernanceEnvelope`, `GovernanceMetadata`, `GovernanceL1`, `GovernanceL2`, `GovernanceL2Vote`, `GovernanceL3`, `GovernanceL3Proof`, and `compute_transaction_hash` utility implementing the SHA-256 transaction hash algorithm |
| `g8e.models.internal_api` | `ChatMessageRequest`, `ChatStartedResponse`, `ResourceCreationRequest`, `LLMOverrides` |
| `g8e.models.settings` | `G8eeUserSettings`, `PlatformSettings`, and nested settings models for LLM providers, search, eval judge, command validation, and batch execution |

### Python Usage Examples

Example scripts are in `protocol/python/examples/`:

#### Constants Example

`protocol/python/examples/constants_example.py` demonstrates loading event, status, and collection constants, using the `ComponentName` enum, and building HTTP header dicts from the exported header string constants.

Run it from `protocol/python` after `pip install -e .` with `python examples/constants_example.py`.

#### Models Example

`protocol/python/examples/models_example.py` demonstrates constructing a `RequestContext` with bound operators, serializing it to a JSON dict, creating `PlatformSettings` with governance flags, and exercising `RequestContext` validation for the `CLIENT` source component.

Run it from `protocol/python` after `pip install -e .` with `python examples/models_example.py`.

### Python Environment Configuration

The constants loader resolves the protocol directory in the following order:

1. **`G8E_PROTOCOL_DIR`** environment variable: if set, uses `$G8E_PROTOCOL_DIR/constants`
2. **Package bundled data**: `g8e/_data/` within the installed package (for PyPI installs)
3. **Relative path**: `protocol/constants/` relative to the package (for development checkouts)
4. **Container fallback**: `/app/protocol/constants` (for Docker/containerized deployments)

Override the default:

```bash
export G8E_PROTOCOL_DIR=/custom/path/to/protocol
```

---

## Shared Protocol Assets

### Constants Registries

The `protocol/constants/` directory contains JSON files that serve as the authoritative source for protocol identifiers, paths, and configuration values across all g8e components. Both the Go and Python packages consume these same JSON files.

| File | Description |
|------|-------------|
| `events.json` | Event type identifiers for all governance, operator, and platform events |
| `status.json` | Status code definitions for operator sessions, executions, and components |
| `collections.json` | Database collection names |
| `api_paths.json` | API endpoint paths |
| `auth.json` | Authentication-related constants |
| `headers.json` | HTTP header names for session, context, and identity propagation |
| `kv_keys.json` | Key-value store key names |
| `channels.json` | Pub/sub channel names |
| `pubsub.json` | Pub/sub event type definitions |
| `intents.json` | Intent classification identifiers |
| `prompts.json` | Prompt template identifiers |
| `agents.json` | Agent role definitions |
| `platform.json` | Platform-wide configuration constants |
| `senders.json` | Message sender identifiers |
| `exit_codes.json` | Process exit code definitions |
| `field_paths.json` | Structured field path identifiers |
| `document_ids.json` | Document type identifiers |
| `network.json` | Network-related constants |
| `output.json` | Output format constants |
| `ports.json` | Default port assignments |
| `timestamp.json` | Timestamp format constants |
| `env_vars.json` | Environment variable names |
| `doctrine/` | L1 Doctrine pattern registries (`doctrine_registry.json`, `gitleaks_doctrine.json`, `mcp_vectors_doctrine.json`, `owasp_crs_doctrine.json`, `blacklist_doctrine.json`, `whitelist_doctrine.json`) |

### JSON Model Schemas

The `protocol/models/` directory contains JSON Schema files that define the structure for protocol data structures, including: `account_lock.json`, `app_policy.json`, `bound_session.json`, `case.json`, `cli_session.json`, `conversation.json`, `operator_session.json`, `organization.json`, `passkey_challenge.json`, `persona.json`, `platform_settings.json`, `reputation_commitment.json`, `security_constraints.json`, `task.json`, `consensus.json`, `user.json`, `web_session.json`, and more. The directory also contains `errors.py`, which defines Python error code and category enums.

Per-agent-role model schemas are in `models/agents/` (`primary.json`, `assistant.json`, `lite.json`, `triage.json`, `title_generator.json`, `agent_harness.json`).

### MCP Server Example Configurations

Example MCP server configurations are in `protocol/examples/mcp_server/`:

- **`g8e_gateway_mcp_config.json`**: HTTP + mTLS MCP server configuration with literal cert paths for production deployments. Uses the unified `/mcp` endpoint on port 8443.
- **`g8e_gateway_mcp_config_env.json`**: HTTP + mTLS MCP server configuration with env-var cert paths for containerized deployments.
- **`g8e_stdio_mcp_config.json`**: Stdio MCP server configuration for local development with native tools. Requires a running gateway; the g8e binary proxies all requests to the gateway over mTLS. Compatible with Claude Code, Codex, Goose, Gemini CLI, and other MCP-compatible clients.
- **`g8e_agent_mcp_config.json`**: Agent MCP config example demonstrating the format written by `g8e mcp agent run` to agent-specific config files. Disables native agent tools to force all I/O through g8e's governed MCP tools so every action is audited at L1-L5. The tool-disabling mechanism varies by agent: `excludeTools` for Claude/Codex, `tools.core: []` for Gemini, `--no-profile` with `--with-extension` for Goose.

---

## Protobuf Code Generation

Protobuf code generation uses [buf](https://buf.build). The generation config is in `buf.gen.yaml` at the repository root.

### Prerequisites

Install buf and protoc plugins:

```bash
make buf-install      # installs buf v1.70.0
make protoc-install   # installs protoc compiler (optional; buf ships its own)
```

### Generating Code

From the repository root:

```bash
buf generate
```

This produces:

| Output | Destination | Plugin |
|--------|-------------|--------|
| Go structs | `protocol/proto/` | `protoc-gen-go` (`paths=source_relative`) |
| gRPC Go stubs | `protocol/proto/` | `protoc-gen-go-grpc` (`paths=source_relative`) |
| Markdown API docs | `protocol/docs/reference/api/` | `protoc-gen-doc` |

The buf module config is in `protocol/proto/buf.yaml` (module: `buf.build/g8e/platform`).

---

## Release & Distribution

### Unified Versioning

The protocol (Go + Python) and the platform binary share a single version number. There are no separate protocol releases. The current version is tracked in the `VERSION` file at the repository root.

Versioning follows [Semantic Versioning](https://semver.org/spec/v2.0.0.html):

- **MAJOR** (X.0.0): Breaking protocol changes that require consumer action
- **MINOR** (x.Y.0): New protocol features, backward-compatible
- **PATCH** (x.y.Z): Bug fixes and non-breaking enhancements

Version string conventions:

- **With `v` prefix** (`vX.Y.Z`): `VERSION` file, git tags, doc `Version:` headers
- **Without `v` prefix** (`X.Y.Z`): `CHANGELOG.md`, Python package version (`pyproject.toml`, `__init__.py`)

### Release Workflow

The release process is automated through a single Make target:

#### `make release`: Sync, Tag & Push

1. Syncs `protocol/python/pyproject.toml` and `protocol/python/g8e/__init__.py` from `VERSION`
2. Verifies working tree is clean after version sync
3. Verifies release notes file exists at `docs/release_notes/vX.Y.x/vX.Y.Z.md`
4. Verifies tags `vX.Y.Z` and `protocol/vX.Y.Z` do not already exist
5. Creates `vX.Y.Z` and `protocol/vX.Y.Z` tags on the current commit
6. Pushes both tags to origin

Binary building, GitHub release creation, and asset uploading are handled by CI workflows triggered by the tag pushes.

### CI Workflows

Pushing the tags triggers two CI workflows:

| Tag | Workflow File | What It Does |
|-----|---------------|-------------|
| `vX.Y.Z` | `.github/workflows/release-binary.yml` | Builds platform binaries for all OS/arch combinations, signs them with cosign, creates GitHub release with binary assets, SHA256 checksums, signatures, and release notes from the notes file. A verify-install job confirms `go install` works on Ubuntu, macOS, and Windows. |
| `protocol/vX.Y.Z` | `.github/workflows/release-python-protocol.yml` | Builds Python sdist + wheel, publishes to [PyPI](https://pypi.org/project/g8e/). A verify-install job confirms fresh PyPI install and import on Ubuntu, macOS, and Windows. |

#### Python Protocol CI Details

The Python protocol workflow (`.github/workflows/release-python-protocol.yml`):

1. Checks out the repository with full history
2. Sets up Python 3.14
3. Validates package metadata (name, version, license, authors, classifiers, URLs)
4. Copies protocol constants into `protocol/python/g8e/_data/` for bundled distribution
5. Builds the package: `python -m build` (produces sdist + wheel in `protocol/python/dist/`)
6. Validates with `twine check dist/*`
7. Publishes to PyPI using trusted publishing (OIDC `id-token: write`)
8. Verifies fresh PyPI install and imports on Ubuntu, macOS, and Windows

### Version Sync Enforcement

The `make release` target verifies that `pyproject.toml` and `__init__.py` versions match `VERSION` before creating tags. A mismatch will abort the release.

Additionally, `.github/workflows/build-and-test.yml` includes a version sync check that fails CI if `pyproject.toml` or `__init__.py` don't match `VERSION`.

The Go module version is not stored in a file; it is derived entirely from git tags (`vX.Y.Z`). The protocol code is part of the root module and does not have a separate `go.mod`.

---

## Conformance Tests

Cross-language conformance tests in `protocol/conformance/` validate that protocol constants and models are identical across the Python package and Go implementation. The JSON files in `protocol/constants/` and `protocol/models/` are the single source of truth; both Go code and the Python package consume them.

- **`test_constants.py`**: Verifies JSON structural integrity, `_go_const` and `_python_const` field presence, value uniqueness, event namespace conventions, and Python-JSON parity for all 20+ protocol constant files.
- **`test_models.py`**: Verifies model schema integrity, `PlatformSettings` field parity between Python and JSON schema, `RequestContext` validation rules, and serialization round-trip behavior.
- **`test_hash_parity.py`**: Verifies cross-language transaction hash parity using shared test vectors from `hash_vectors.json`. Validates that Python's `compute_transaction_hash` produces identical SHA-256 digests to Go's `GenerateMessageID` for the same envelope fields, including timestamp normalization and optional field handling.

A `conformance` CI job in `.github/workflows/build-and-test.yml` runs these tests on every push and PR using Python 3.14. See [Conformance Tests](../../protocol/conformance/README.md) for details.

---

## Directory Structure

```
protocol/
  go_package.go              Package doc for the Go protocol package
  workload_identity.go       SPIFFE workload identity generation and validation
  workload_identity_test.go  Unit tests for workload identity helpers
  Makefile                   Test, format, vet, lint, and OpenAPI targets
  LICENSE                    Apache 2.0 license
  proto/                     Protobuf schema definitions and generated Go code
    buf.yaml                 Buf module config (buf.build/g8e/platform)
    g8e/common/v1/           Common governance types (common.proto, common.pb.go)
    g8e/operator/v1/         Operator service types (operator.proto, operator.pb.go, operator_grpc.pb.go)
    g8e/pubsub/v1/           Pub/sub message types (pubsub.proto, pubsub.pb.go)
  constants/                 JSON protocol constant registries
    doctrine/                L1 Doctrine pattern registries
  models/                    JSON model schemas
    agents/                  Per-agent-role model schemas
    errors.py                Python error code and category enums
  conformance/               Cross-language conformance tests
    test_constants.py        Constant file structural integrity and parity tests
    test_models.py           Model schema parity and validation tests
    test_hash_parity.py      Transaction hash parity tests (Python vs Go)
    hash_vectors.json        Shared test vectors for hash parity tests
  python/                    Python package (g8e)
    g8e/                     Import namespace (g8e)
      __init__.py            Package version
      constants.py           Runtime loader for JSON protocol constants
      enums.py               Dynamic StrEnum/IntEnum generation from STATUS, EVENTS, CHANNELS, INTENTS, PROMPTS, COLLECTIONS, KV
      py.typed               PEP 561 marker for type-checker support
      _data/                 Bundled JSON protocol constants (for PyPI installs)
      models/                Pydantic v2 models for protocol data structures
        governance.py        GovernanceEnvelope model and transaction hash utility
    tests/                   Python package unit tests
    examples/                Python example scripts
    pyproject.toml           Package metadata and dependencies
    README.md                Python package README
  examples/                  Go example programs
    governance_envelope/     GovernanceEnvelope construction demo
    workload_identity/       SPIFFE workload identity demo
    mcp_server/              Example MCP server configurations
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

---

## References

- [Protocol Specification](../../protocol/docs/spec.md): GovernanceEnvelope structure and 5-layer interlock sequence
- [Constants Reference](../../protocol/docs/constants.md): Constants system
- [A2A Protocol](../../protocol/docs/a2a.md): A2A protocol integration
- [MCP Protocol](../../protocol/docs/mcp.md): MCP protocol integration
- [Release Process](../devs/release_process.md): Full release checklist and version management
- [Semantic Versioning](https://semver.org/spec/v2.0.0.html)
- [Go Module Versioning](https://go.dev/doc/modules/versioning)
- [PyPI Packaging](https://packaging.python.org/en/latest/tutorials/packaging-projects/)
- [Buf Documentation](https://buf.build/docs/)
