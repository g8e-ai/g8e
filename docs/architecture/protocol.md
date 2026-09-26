---
title: g8e Protocol Library
parent: Architecture
---

# g8e Protocol Library

Last Updated: 2026-09-24
Version: v2.1.13

The g8e Protocol Library is the canonical wire contract for governed operations that enter the platform through a g8e ingress. It provides protobuf schemas and generated bindings, JSON constant registries, JSON model schemas, Python Pydantic models, canonicalization and verification helpers, SPIFFE workload identity helpers, and examples for compatible clients and services. Governed mutations use the five-layer interlock; discovery, read-only operations, and the external MCP wrapper have narrower behavior documented in [AI Agents and the g8e Governance Boundary](./agents.md). The event vocabulary, transport ownership, and registry contract are defined in [Event and Action Protocol](./events.md).

- **L1 Doctrine**: Hard gates, forbidden-pattern matching, and threat detection.
- **L2 Consensus**: K-of-N Ed25519 protocol authorization over the transaction hash when required by posture.
- **L3 Notary**: Human authorization through WebAuthn or signed CLI proofs when required by posture.
- **L4 Warden**: Pre-dispatch verification of integrity, replay protection, expiry, nonce, state binding, and required proofs.
- **L5 Actuator**: Governed dispatch, just-in-time capability minting, and signed receipt production.

The repository contains the Go protocol packages in the root module, the installable Python package, and private generated TypeScript protobuf artifacts for repository consumers. These surfaces share the version in `VERSION`; the Go package is released through the root module tag, while the Python package is published from a separate `protocol/v*` release tag. The TypeScript artifacts are not a published package.

---

## Table of Contents

- [Go Protocol Package](#go-protocol-package)
  - [Requirements & Installation](#go-requirements--installation)
  - [Module Path & Versioning](#go-module-path--versioning)
  - [Package Overview](#go-package-overview)
  - [Workload Identity Helpers](#go-workload-identity-helpers)
  - [Development & Tooling](#go-development--tooling)
- [Python Protocol Package](#python-protocol-package)
  - [Requirements & Installation](#python-requirements--installation)
  - [Package Overview](#python-package-overview)
  - [Constants & Enums](#python-constants--enums)
  - [Pydantic Models](#python-pydantic-models)
  - [Environment Configuration](#python-environment-configuration)
- [TypeScript Protobuf Package](#typescript-protobuf-package)
- [Shared Protocol Assets](#shared-protocol-assets)
  - [Constants Registries](#constants-registries)
  - [JSON Model Schemas](#json-model-schemas)
  - [MCP Server Configurations](#mcp-server-configurations)
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

### Go Requirements & Installation

The Go protocol package requires Go 1.26.6 or later. The root module directly depends on `google.golang.org/grpc v1.84.0` and `google.golang.org/protobuf v1.36.12`; remaining dependencies are managed through the root `go.mod`.

Install or update the Go module using standard Go tooling:

```bash
go get github.com/g8e-ai/g8e/v2@v2.1.13
```

To fetch the latest release:

```bash
go get github.com/g8e-ai/g8e/v2@latest
```

### Go Module Path & Versioning

The Go protocol implementation belongs to the root module `github.com/g8e-ai/g8e/v2`. There is no separate module file for the protocol directory. Module versions derive from git tags of the form `vX.Y.Z`, created during the release workflow. The Go module proxy resolves release versions directly from these tags.

Import paths use `github.com/g8e-ai/g8e/v2/protocol/...`. Consumers configure their module requirements by referencing the root module `github.com/g8e-ai/g8e/v2 vX.Y.Z`. See [Release & Distribution](#release--distribution) for version sync details.

### Go Package Overview

The Go protocol package provides the canonical wire structures and helpers for platform interaction. Generated bindings for the other supported language surfaces are maintained alongside it in the repository.

- **Generated Protobuf Types**: Contains compiled Go structs and gRPC client stubs generated from protobuf schemas in `protocol/proto/g8e/...`; generated Python modules and type stubs live under `protocol/python/g8e/`, and private generated TypeScript modules live under `protocol/node/src/gen/`.
- **Governance Types**: Provides structures for the canonical transaction envelope, governance metadata, consensus votes, deterministic governance-stage evidence, persistence attestations, and threat pattern options.
- **Compliance Types**: Provides canonical assertion, framework, crosswalk, assessment-scope, content-addressed evidence, assessment, report-manifest, verification-report, demo-manifest, scenario, step-result, scenario-result, and metric-evidence messages in `g8e.compliance.v1`, with deterministic protojson helpers in Go and Python.
- **Evaluation Types**: Provides eval-native campaign, provider-boundary observation, model-provenance, bundle-verification, and publication messages in `g8e.eval.v1`.
- **Operator Services**: Defines gRPC service interfaces for command execution, file modifications, filesystem inspection, and governance verification.
- **Pub/Sub and Event Types**: Defines pub/sub envelopes, event transport, and result messages for distribution across nodes.
- **Receipt Verification**: Defines canonical receipt and persistence-attestation serialization and Ed25519 verification in Go and in `g8e.receipts`, backed by shared cross-language vectors.

### Go Workload Identity Helpers

Workload identity helpers manage SPIFFE identity generation and validation for the `g8e.local` trust domain (`protocol/workload_identity.go`). Six workload identity types are supported:

- **Operator**: Identifies target execution node instances (`spiffe://g8e.local/operator/<org_id>/<operator_id>/<session_id>`).
- **CLI**: Identifies authenticated command line interface sessions (`spiffe://g8e.local/cli/<user_id>/<session_id>`).
- **App**: Identifies external application and agent integrations (`spiffe://g8e.local/app/<operator_id>`), including the centralized ensemble broker identity `spiffe://g8e.local/app/g8ee` (`EnsembleAppID`). See [Ensemble PKI & Trust](../ensemble/pki.md) for how g8ee uses this identity.
- **User**: Identifies human user sessions (`spiffe://g8e.local/user/<user_id>`).
- **Hub**: Identifies central gateway listener endpoints (`spiffe://g8e.local/hub/operator-listen`).
- **GatewayPeer**: Identifies peer gateway nodes in distributed setups (`spiffe://g8e.local/gateway/<gateway_id>`).

Workload helpers format SPIFFE identifiers and URLs, validate incoming request identities against expected trust domain schemes (`MatchesOperator`, `MatchesCLI`, `MatchesApp`, `MatchesHub`, `MatchesGatewayPeer`), and inspect identity prefixes (`IsAppSAN`, `IsUserSAN`, `IsEnsembleApp`). Session validation methods like `MatchesCLISessionOnly` verify CLI identities during initial authentication before loading broader user context. Extraction methods retrieve component IDs directly from SPIFFE strings (`ExtractCLISessionID`, `ExtractUserID`, `ExtractUserIDFromUserSAN`, `ExtractOperatorSessionID`, `ExtractGatewayID`).

### Go Development & Tooling

Protocol development uses standard Make targets defined in the protocol build configuration (`protocol/Makefile`):

- `make test`: Runs Tier 1 unit tests, then Tier 2 integration tests with race detection (`go test -tags=integration -race -count=1 ./...`).
- `make fmt`: Formats source files using standard formatting rules (`gofmt -s -w .`).
- `make vet`: Executes Go static analysis checks (`go vet ./...`).
- `make lint`: Runs configured linter checks across the package (`golangci-lint run`).
- `make openapi`: Stub target that prints setup instructions; it does not generate an OpenAPI document. Use the root `make proto` target to regenerate protobuf artifacts.

---

## Python Protocol Package

### Python Requirements & Installation

The Python package requires Python 3.10 or later (tested on 3.10 through 3.14). Runtime dependencies are `pydantic>=2.0.0`, `protobuf>=4.0.0`, and `PyNaCl>=1.5.0`. The build system uses `setuptools>=61.0` and `wheel`; development dependencies include `grpcio-tools==1.84.0` and `pytest>=8.0`.

Install the package from PyPI using pip:

```bash
pip install g8e
```

To pin a specific release version:

```bash
pip install g8e==2.1.13
```

### Python Package Overview

The Python package installs as `g8e` and provides generated protobuf modules and type stubs, including `g8e.compliance.v1` and `g8e.eval.v1`, type-checked models, runtime constants, canonical compliance and evaluation protojson helpers, and canonical `ActionReceipt` parsing and Ed25519 verification helpers for Python applications. It includes standard type markers (`py.typed`) for static type-checker support. Unit tests cover constant loading, enum generation, model validation, receipt and persistence-attestation verification, and cross-language parity.

### Python Constants & Enums

The constants module (`g8e.constants`) loads protocol JSON constant files at import time and fails closed via `ProtocolConstantsError` if any constant file is missing, empty, or malformed. It exports dictionaries for `EVENTS`, `STATUS`, `MSG`, `COLLECTIONS`, `KV`, `CHANNELS`, `PUBSUB`, `INTENTS`, `PROMPTS`, `TIMESTAMP`, `HEADERS`, `DOCUMENT_IDS`, `PLATFORM`, `AGENTS`, `NETWORK`, and `API_PATHS`. It also exports component names (`ComponentName`: `CLIENT`, `G8EO`, `G8EO_GATEWAY`), HTTP header name constants matching Go source of truth, and lookup helpers (`collection()`, `channel()`, `document_id()`, `intent()`, `prompt()`, `kv_key()`, `kv_session_type()`).

Dynamic enum generation (`g8e.enums`) builds string and integer enums from the underlying constant registries. Key features include:

- Member names use uppercase snake case (SCREAMING_SNAKE_CASE) formatting.
- Values preserve raw wire format strings and integer status codes.
- Integer categories (`citation_layout`, `priority`, `scrubber_priority`, `severity`, `slash_tier`) produce `IntEnum`, while text categories produce `StrEnum`.
- Exports `EventType` generated from `EVENTS`, all status category enums from `STATUS`, and non-STATUS enums (`Channel`, `Intent`, `Prompt`, `Collection`, `KVKey`).
- Enums are evaluated lazily upon access and cached in memory using `lru_cache`.

### Python Pydantic Models

Pydantic v2 models in `g8e.models` define protocol structures with typed field validation. Most models extend `G8eBaseModel` (which configures `populate_by_name=True`, `extra="ignore"`, and serializes UTC datetimes to standard ISO 8601 strings with a `Z` suffix); selected producer and public-feed models override this with `extra="forbid"`.

- **Request Context (`g8e.models.context`)**: Validates session identity and operator bindings for client requests (`RequestContext`, `BoundOperator`).
- **Internal API Models (`g8e.models.internal_api`)**: Defines payloads for chat sessions, message streaming, and LLM overrides (`ChatMessageRequest`, `ChatStartedResponse`, `ResourceCreationRequest`, `LLMOverrides`).
- **Event Models (`g8e.models.events`)**: Defines event payload structures for streaming session events, background execution, tool lifecycles, thinking phases, chat completions, and observe telemetry (`SessionEventWire`, `BackgroundEventWire`, `AiProcessingStoppedPayload`, `AIToolLifecyclePayload`, `ChatCitationsReadyPayload`, `ChatErrorPayload`, `ChatProcessingStartedPayload`, `ChatResponseChunkPayload`, `ChatResponseCompletePayload`, `ChatRetryPayload`, `ChatThinkingPayload`, `ChatTurnCompletePayload`, `TriageClarificationQuestionsPayload`, `AgentStatusUpdatedPayload`, `RunStatusUpdatedPayload`, `EvalRunCompletedPayload`, `EvalMetricRecordedPayload`, `ObservedMeasurement`).
- **Observe API Models (`g8e.models.observe_api`)**: Defines browser-facing read projections and mTLS producer request/response types for the observe API (`ObserveBootstrapSnapshot`, `RunDetail`, `EvalDetail`, `DownloadArtifact`, `ObserveProducerAgentStateRequest`, `ObserveProducerRunStateRequest`, `ObserveProducerResponse`). Producer models use `extra="forbid"` for unknown-field rejection.
- **Public Feed Models (`g8e.models.public_feed`)**: Defines outbound public-spectator batch, snapshot, ingest, proof-catalog, and cursor-page types (`PublicFeedBatch`, `PublicFeedSnapshot`, `PublicFeedBootstrap`, `PublicIngestRequest`, `PublicProofManifest`).
- **Governance Envelope (`g8e.models.governance`)**: Represents the canonical transaction container, carrying identity, intent, state Merkle roots, and governance proofs (`GovernanceEnvelope`, `GovernanceMetadata`, `GovernanceL1`, `GovernanceL2`, `GovernanceL2Vote`, `GovernanceL3`, `GovernanceL3Proof`, `CommandIntent`, `compute_transaction_hash`).
- **Settings Models (`g8e.models.settings`)**: Represents platform and user configuration parameters for search, evaluation judges, command validation, batch execution, and execution limits (`PlatformSettings`, `G8eeUserSettings`, `LLMSettings`, `SearchSettings`, `EvalJudgeSettings`, `CommandValidationSettings`, `BatchExecutionSettings`).

### Python Environment Configuration

The constant loader resolves the protocol definition directory using a two-step fail-closed sequence:

1. `G8E_PROTOCOL_DIR` environment variable: Uses the specified filesystem path if set (appending `/constants` to load registries). If set to an empty string, it is treated as unset.
2. Bundled package data: Uses packaged JSON data in `g8e/_data/` included with package installations and containers.

There are no unvalidated probe fallbacks for checkout or container paths. If constant files cannot be found or are malformed, initialization fails closed with `ProtocolConstantsError`.

Override the resolution directory during development by setting the environment variable:

```bash
export G8E_PROTOCOL_DIR=/custom/path/to/protocol
```

---

## TypeScript Protobuf Package

`protocol/node/` contains private generated TypeScript protobuf bindings for repository consumers. It is not published to npm. The bindings use `@bufbuild/protobuf` and cover the common, compliance, evaluation, operator, and pub/sub schemas. Generate them with `make proto-node` from the repository root and run `npm ci --prefix protocol/node && npm --prefix protocol/node run typecheck` to verify the generated modules.

---

## Shared Protocol Assets

### Constants Registries

JSON files in `protocol/constants/` are external protocol definitions and reference data for identifiers, endpoint paths, and default configurations. Python loads the registries from bundled package data; Go constants have their own source declarations, and contract tests keep mirrored values aligned. Registry ownership is therefore specific to each registry and is not uniformly shared by both language packages.

Registries cover event names (`events.json`), status codes (`status.json`), database collections (`collections.json`), API paths (`api_paths.json`), authentication parameters (`auth.json`), HTTP headers (`headers.json`), key-value keys (`kv_keys.json`), channels (`channels.json`), pubsub definitions (`pubsub.json`), intents (`intents.json`), prompt templates (`prompts.json`), agent roles (`agents.json`), platform settings (`platform.json`), platform enrollment parameters and transcript vectors (`platform_enrollment.json`, `platform_enrollment_completion_transcript_vectors.json`), compliance artifact paths and digest-verified assertion, framework, crosswalk, and demo-scenario catalogs (`compliance_paths.json`, `compliance/`), senders (`senders.json`), exit codes (`exit_codes.json`), field paths (`field_paths.json`), document types (`document_ids.json`), network parameters (`network.json`), output formats (`output.json`), default ports (`ports.json`), timestamp formats (`timestamp.json`), and environment variable names (`env_vars.json`). Threat detection pattern registries in `protocol/constants/doctrine/` define forbidden execution patterns, blacklist/whitelist rules, Gitleaks patterns, OWASP CRS rules, and MCP attack vector patterns for L1 Doctrine evaluation.

The event dashboard classification inventory (`protocol/constants/event_dashboard_classification.json`) classifies every registered event family by its relationship to the browser observability frontend: `produced_to_sse` (a real production path emits this event through the SSE push endpoint with a typed payload), `governed_record_only` (persisted but not emitted as browser telemetry), `mixed` (some events in the family are produced to SSE, others are governed records), and `unsupported` (registered but no real producer or no protocol-owned payload schema). The `g8e.v1.app.agent` family is `mixed` because `app.agent.activity.recorded` is `governed_record_only` while `app.agent.status.updated` is `produced_to_sse` through the Gateway mTLS producer endpoint. The `g8e.v1.app.run` family is `produced_to_sse` because `app.run.status.updated` is produced through the Gateway mTLS producer endpoint and verified by real HTTP integration tests. The `g8e.v1.ai.eval` family is `produced_to_sse` and `dashboard_safe`: live campaign events (`ai.eval.cycle.started`, `ai.eval.assignment.completed`, `ai.eval.publication.completed`, and related types) are emitted by the in-process observe producer after persisting projections; gateway live tests verify emission. Reserved dashboard contracts `ai.eval.run.completed` and `ai.eval.metric.recorded` remain typed payloads without a native evaluation HTTP producer in the current implementation. The `g8e.v1.public.feed` family is `produced_to_sse` for outbound public-spectator export events. Coverage and value tests in `internal/constants/event_dashboard_classification_test.go` enforce that classifications correspond to actual producer evidence.

### JSON Model Schemas

JSON Schema files in `protocol/models/` define structural validation rules for data structures across the platform. Managed schemas include account locks, agent activity metadata, application policies, approvals, authentication administrative audits, bound sessions, cases, chat messages, CLI sessions, consensus configurations, console audits, conversations, conversation messages, enrollment tokens, execution results, file edits, filesystem grep/list operations, governance containers, heartbeats, investigations, local OS users, login audits, memories, observe API read and producer models, observe event payloads, operator documents, operator sessions, operator usage records, organizations, passkey challenges, passkey credentials, personas, platform enrollments, platform settings, public spectator feed batches and proofs, reputation commitments, reputation states, request contexts, revoked certificates, runtime configurations, security constraints, SSE event payloads, SSE event wire representations, SSE push payloads, stake resolutions, tasks, terminal outputs, tool results, trusted signers, users, user settings, web sessions, and WebAuthn responses. Python error category and code definitions are defined in `protocol/models/errors.py`. Authenticated third-party schemas live under `protocol/schemas/`, including the pinned NIST OSCAL 1.1.2 assessment-results schema and provenance metadata.

The observe API models (`protocol/models/observe_api.json`) define the browser-facing read models (`ObserveBootstrapSnapshot`, `ObservePage`, `RunDetail`, `EvalDetail`, `DownloadArtifact`) and the mTLS-internal producer request/response models (`observe_producer_agent_state_request`, `observe_producer_run_state_request`, `observe_producer_response`). The producer request models use typed lifecycle enums (`AgentLifecycleStatus`, `RunLifecycleStatus`, `RunKind`) and require exactly one routing target (`web_session_id` or `cli_session_id`). The producer response is a typed `{ "accepted": true }` object. The Python protocol package (`protocol/python/g8e/models/observe_api.py`) provides `ObserveProducerAgentStateRequest`, `ObserveProducerRunStateRequest`, and `ObserveProducerResponse` with `extra="forbid"` for unknown-field rejection.

The observe event payloads (`protocol/models/observe_event_payloads.json`) define dashboard and campaign event payloads, including `app.agent.status.updated`, `app.run.status.updated`, `ai.eval.run.completed`, `ai.eval.metric.recorded`, and the live campaign types (`ai.eval.cycle.started`, `ai.eval.assignment.completed`, `ai.eval.publication.completed`, and related `g8e.v1.ai.eval.*` payloads).

Per-agent role schemas in `protocol/models/agents/` define tailored models for primary (`primary.json`), assistant (`assistant.json`), lite (`lite.json`, displayed as Lite), triage (`triage.json`), title generator (`title_generator.json`), and agent harness (`agent_harness.json`) roles.

### MCP Server Configurations

Example Model Context Protocol (MCP) server configurations in `protocol/examples/mcp_server/` demonstrate deployment topologies:

- **Gateway mTLS Configuration (`g8e_gateway_mcp_config.json`)**: HTTP with mTLS using certificate paths for production deployments.
- **Containerized mTLS Configuration (`g8e_gateway_mcp_config_env.json`)**: HTTP with mTLS using environment variables (`G8E_CLIENT_CERT`, `G8E_CLIENT_KEY`, `G8E_CA_BUNDLE`) for container environments.
- **Stdio Configuration (`g8e_stdio_mcp_config.json`)**: Direct stdio execution of `g8e mcp stdio` for local development.
- **Agent Governance Configuration (`g8e_agent_mcp_config.json`)**: Enforces L1 through L5 governance by excluding ungoverned native agent tools (`Bash`, `Read`, `Write`, `Edit`, `Glob`, `Grep`, `WebSearch`, `WebFetch`) and routing tool operations through governed gateway endpoints.

---

## Protobuf Code Generation

Protobuf code generation is managed with `buf`. Generation behavior is configured in the root code generation configuration file `buf.gen.yaml`. The protobuf module configuration resides in `protocol/proto/buf.yaml` (`buf.build/g8e/platform`).

Compile schemas from the repository root using the Buf-based `proto` target:

```bash
make proto
```

`make proto` installs the Buf CLI if it is not present and then runs `buf generate protocol/proto` to produce Go structs, gRPC stubs, and Markdown API reference documentation in `protocol/docs/reference/api`. From `protocol/`, `make python-proto` regenerates the Python protobuf modules and `.pyi` type stubs, while `make proto-check` verifies that committed Python outputs match the schemas. The root `make proto-node` target regenerates private TypeScript protobuf bindings under `protocol/node/src/gen/`.

---

## Release & Distribution

### Unified Versioning

The protocol packages and platform binary share a single version number tracked in the `VERSION` file at the repository root. Versioning follows Semantic Versioning guidelines:

- **MAJOR**: Breaking protocol changes requiring client updates.
- **MINOR**: Backward-compatible new protocol features.
- **PATCH**: Backward-compatible bug fixes and minor updates.

Version strings use a `v` prefix (`v2.0.0`) in tags, repository version files, and documentation headers. Python package metadata (`pyproject.toml`, `__init__.py`) and CHANGELOG entries omit the prefix (`2.0.0`).

### Release Workflow

Releases are tagged and published through the release orchestration target:

```bash
make release
```

The release target executes the following sequence:

1. Syncs Python package metadata files (`protocol/python/pyproject.toml` and `protocol/python/g8e/__init__.py`) with the current `VERSION` file.
2. Confirms that the working tree contains no uncommitted changes.
3. Verifies that the release notes file exists for the targeted version (`docs/release_notes/vX.Y.x/vX.Y.Z.md`).
4. Confirms that git tags for the platform (`vX.Y.Z`) and protocol (`protocol/vX.Y.Z`) do not already exist.
5. Creates platform and protocol git tags on the current commit.
6. Pushes both tags to the remote repository origin.

### CI Workflows

Tag pushes trigger automated GitHub Actions release pipelines:

- **Binary Release Workflow**: Triggered by `v*` tags. Builds platform binaries for supported platforms and architectures, generates SHA-256 checksums, signs assets with cosign, and creates GitHub releases.
- **Python Protocol Workflow**: Triggered by `protocol/v*` tags. Validates package metadata, bundles JSON constant registries into `_data/`, builds source distributions and wheels, uploads packages to PyPI via trusted publishing, and verifies installation.

### Version Sync Enforcement

The release task verifies version alignment between Python package metadata and the root `VERSION` file before creating git tags. CI test workflows enforce version alignment on pull requests and pushes, failing the build if package versions diverge. Go module versions derive directly from git tags without manual version file duplication.

---

## Conformance Tests

Cross-language conformance tests validate that constants and models remain identical across Go and Python implementations. Test scripts in `protocol/conformance/` verify:

- Constant file structural integrity, `_go_const` and `_python_const` field presence, and uniqueness rules across all registries (`test_constants.py`).
- Model schema alignment and validation rule enforcement between Pydantic models and JSON schemas (`test_models.py`).
- SHA-256 transaction hash parity between Python (`compute_transaction_hash`) and Go (`GenerateMessageID`) implementations using shared test vector files (`test_hash_parity.py` using `hash_vectors.json`).

Run the conformance suite using the protocol Python development environment from the repository root:

```bash
uv run --project protocol/python --extra dev pytest protocol/conformance -v
```

The Python package tests use the same environment:

```bash
uv run --project protocol/python --extra dev pytest protocol/python/tests -v
```

For the generated TypeScript protobuf package, run:

```bash
npm ci --prefix protocol/node
npm --prefix protocol/node run typecheck
```

---

## Directory Structure

The protocol implementation is organized into functional subdirectories within the repository:

- `protocol/`: Root directory containing package documentation and SPIFFE workload identity helpers (`go_package.go`, `workload_identity.go`).
- `protocol/proto/`: Protobuf schema definitions and generated Go code for common, compliance, evaluation, operator, and pub/sub packages (`buf.yaml`, `g8e/*/v1/`).
- `protocol/constants/`: JSON protocol constant registries, compliance catalogs, and L1 Doctrine threat detection patterns in `doctrine/`.
- `protocol/models/`: JSON Schema definitions for platform models, per-agent role schemas in `agents/`, and Python error enums (`errors.py`).
- `protocol/schemas/`: Authenticated third-party schemas and provenance metadata used for offline validation, including NIST OSCAL.
- `protocol/vectors/`: Cross-language canonicalization and verification examples for receipts, persistence attestations, compliance, and evaluation. These executable examples validate canonical encodings and digests; they do not duplicate or replace protobuf and JSON model definitions.
- `protocol/conformance/`: Cross-language test suites for constant structure, model schema, and transaction-hash parity verification.
- `protocol/python/`: Installable Python package implementation (`g8e`), including constants loaders, dynamic enums, Pydantic models, generated protobuf modules, and bundled package data.
- `protocol/node/`: Private generated TypeScript protobuf modules and their typecheck/test configuration.
- `protocol/examples/`: Sample programs demonstrating envelope construction, workload identity, and MCP server configurations.
- `protocol/docs/`: Protocol specification documents, JSON-RPC schemas, template files, and generated API reference documentation in `reference/api/`.

---

## TypeScript Generation

The deterministic contract pack generator at `dashboard/g8e-adapter/generator/gen-contract-pack.mjs` produces TypeScript models, validators, and fixtures from canonical protocol JSON. It reads `protocol/models/observe_api.json`, `protocol/models/observe_event_payloads.json`, `protocol/constants/event_dashboard_classification.json`, and the compiled adapter dist. It generates `contract-pack/models.ts` with typed interfaces, enums, type guards, and structural validators for all browser-facing read models and event payloads. Inline object models are discovered during resolution and emitted as named interfaces. Producer request/response models are excluded (mTLS-internal). The generated `models.ts` type-checks cleanly under strict TypeScript (`tsc --noEmit` with `strict`, `noUnusedLocals`, `noUnusedParameters`).

The generator is self-contained: it uses only Node built-ins (`node:crypto`, `node:fs/promises`, `node:path`, `node:url`) plus the compiled adapter dist. No new dependencies are introduced. Re-running against identical inputs produces byte-identical files. A `--check` mode regenerates in memory and compares to committed files without rewriting them, exiting non-zero if any output is stale or missing.

See [Generator-Neutral Builder Guide](../guides/build_observe_frontend.md) for the contract pack workflow and acceptance commands.

---

## References

- [Protocol Specification](../../protocol/docs/spec.md): Canonical envelope structure and 5-layer interlock sequence details.
- [Constants Reference](../../protocol/docs/constants.md): Platform constant system details.
- [A2A Protocol](../../protocol/docs/a2a.md): Agent-to-Agent protocol integration guide.
- [MCP Protocol](../../protocol/docs/mcp.md): Model Context Protocol integration guide.
- [Release Process](../devs/release_process.md): Release procedures and version management guidelines.
- [Network Architecture](./network.md): Network topology, mTLS, and SPIFFE identity details.

