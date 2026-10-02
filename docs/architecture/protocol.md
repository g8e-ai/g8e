---
doc_id: protocol
title: Protocol Architecture
audience: maintainers and coding agents
status: current
last_updated: 2026-10-01
version: v2.2.6
owners:
  - protocol/
  - internal/constants/
  - internal/services/governance/
  - buf.gen.yaml
  - Makefile
related:
  - docs/architecture/governance.md
  - docs/architecture/consensus.md
  - docs/architecture/network.md
  - docs/architecture/gateway.md
  - docs/architecture/operator.md
  - docs/architecture/events.md
  - docs/architecture/agents.md
  - docs/devs/docs.md
  - docs/devs/release_process.md
when_to_read: Understanding the g8e wire contract, Go/Python/TypeScript protobuf packages, JSON constant registries and schemas, workload identity (SPIFFE), canonical serialization and verification vectors, and version synchronization.
do_not_use_for:
  - Five-layer governance verification pipeline (docs/architecture/governance.md)
  - L2 machine consensus threshold policies and deliberation (docs/architecture/consensus.md)
  - Platform PKI, TLS/mTLS, and network port topology (docs/architecture/network.md)
  - Event payload taxonomy and streaming transport routing (docs/architecture/events.md)
  - General documentation formatting and maintenance rules (docs/devs/docs.md)
---

# Protocol Architecture

## Purpose

Documents the architecture, wire contracts, code generation pipelines, and verification mechanisms of the g8e Protocol Library. The protocol library establishes the canonical wire contract for all governed transactions entering the platform through a g8e ingress. It provides protobuf schemas and compiled bindings across Go, Python, and TypeScript, authoritative JSON constant registries, JSON Schema validation models, Python Pydantic models, SPIFFE workload identity helpers, deterministic protojson canonicalization, Ed25519 signature verification routines, and cross-language conformance test vectors.

## Quick index

- [Purpose](#purpose)
- [Quick index](#quick-index)
- [Invariants](#invariants)
- [Owned surfaces](#owned-surfaces)
- [Procedures](#procedures)
  - [Multi-language protocol architecture and module structure](#multi-language-protocol-architecture-and-module-structure)
  - [Go protocol package and workload identity helpers](#go-protocol-package-and-workload-identity-helpers)
  - [Python protocol package, constants loader, dynamic enums, and models](#python-protocol-package-constants-loader-dynamic-enums-and-models)
  - [TypeScript generated protobuf package and observe contract generator](#typescript-generated-protobuf-package-and-observe-contract-generator)
  - [Shared protocol assets: constants registries, model schemas, and MCP configurations](#shared-protocol-assets-constants-registries-model-schemas-and-mcp-configurations)
  - [Protobuf compilation and code generation workflow](#protobuf-compilation-and-code-generation-workflow)
  - [Canonical serialization, receipt verification, and cross-language vectors](#canonical-serialization-receipt-verification-and-cross-language-vectors)
  - [Cross-language conformance testing suite](#cross-language-conformance-testing-suite)
  - [Unified versioning, release workflow, and CI pipelines](#unified-versioning-release-workflow-and-ci-pipelines)
- [Anti-patterns](#anti-patterns)
- [Links out](#links-out)

Key invariant groups: [Version Synchronization and Release](#version-synchronization-and-release-inv-prot-ver), [Protobuf and Wire Schemas](#protobuf-and-wire-schemas-inv-prot-schema), [Constants Registries and Schemas](#constants-registries-and-schemas-inv-prot-const), [Workload Identity](#workload-identity-inv-prot-id), [Canonicalization, Receipts, and Vectors](#canonicalization-receipts-and-vectors-inv-prot-canon).

## Invariants

Ids are stable. Append the next free number within each group; do not renumber.

### Version Synchronization and Release (`INV-PROT-VER`)

| ID | Rule |
| --- | --- |
| INV-PROT-VER-01 | The root `VERSION` file is the sole source of truth for platform and protocol version numbers. Python package metadata (`protocol/python/pyproject.toml`, `protocol/python/g8e/__init__.py`, `protocol/python/uv.lock`) and protocol doc headers (`protocol/docs/a2a.md`, `protocol/docs/constants.md`, `protocol/docs/mcp.md`, `protocol/docs/spec.md`) MUST remain in exact string synchronization. |
| INV-PROT-VER-02 | Go protocol packages are part of the root module `github.com/g8e-ai/g8e/v2` and versioned exclusively via root git tags of the form `vX.Y.Z`. The Python package is published to PyPI from a dedicated git tag of the form `protocol/vX.Y.Z`. The `protocol/v*` tag MUST NOT be used for Go module resolution. |
| INV-PROT-VER-03 | The release target `make release` MUST fail closed if the working tree has uncommitted changes after synchronization, if the release notes document `docs/release_notes/v<MAJOR_MINOR>.x/v<VERSION>.md` is missing, or if either `v<VERSION>` or `protocol/v<VERSION>` already exists. |
| INV-PROT-VER-04 | CI pipelines (`.github/workflows/build-and-test.yml`) MUST enforce version alignment across all versioned files on every push and pull request, terminating the build if package or documentation versions diverge from `VERSION`. |

### Protobuf and Wire Schemas (`INV-PROT-SCHEMA`)

| ID | Rule |
| --- | --- |
| INV-PROT-SCHEMA-01 | Protobuf schemas in `protocol/proto/g8e/` are the canonical source of truth for wire structures. Generated code in Go (`protocol/proto/`), Python (`protocol/python/g8e/`), and TypeScript (`protocol/node/src/gen/`) MUST NOT be hand-edited and MUST only be updated through `make proto`. |
| INV-PROT-SCHEMA-02 | Client-facing surfaces (HTTP API, WebSocket pub/sub, receipts, audit exports) MUST carry `GovernanceEnvelope` as canonical protojson. Raw binary protobuf is strictly restricted to internal node storage and private gRPC communications. |
| INV-PROT-SCHEMA-03 | Protobuf code generation MUST use Buf configured via `protocol/proto/buf.yaml` and root `buf.gen.yaml`. The build must rely on `buf generate` rather than requiring a standalone `protoc` compiler binary for Go generation. |

### Constants Registries and Schemas (`INV-PROT-CONST`)

| ID | Rule |
| --- | --- |
| INV-PROT-CONST-01 | JSON files in `protocol/constants/` are the authoritative data for protocol constants. Go constants in `internal/constants/` and Python modules in `protocol/python/g8e/constants.py` MUST mirror the JSON source of truth and be validated by contract tests. |
| INV-PROT-CONST-02 | The Python constants loader (`g8e.constants._get_protocol_dir`) MUST fail closed with `ProtocolConstantsError` if constants cannot be loaded from `G8E_PROTOCOL_DIR` (when set non-empty) or the bundled `_data/` directory. Silent fallbacks to unvalidated filesystem checkout probes are prohibited. |
| INV-PROT-CONST-03 | Every registered event family in `protocol/constants/events.json` MUST be classified in `protocol/constants/event_dashboard_classification.json` (`produced_to_sse`, `governed_record_only`, `mixed`, or `unsupported`), enforced by `internal/constants/event_dashboard_classification_test.go`. |
| INV-PROT-CONST-04 | JSON Schema definitions in `protocol/models/` MUST maintain exact field alignment and type compatibility with corresponding Pydantic models in `protocol/python/g8e/models/`, verified by `protocol/conformance/test_models.py`. |

### Workload Identity (`INV-PROT-ID`)

| ID | Rule |
| --- | --- |
| INV-PROT-ID-01 | All platform workload identities MUST belong to the `g8e.local` trust domain (`protocol.TrustDomain`) and follow one of six canonical URI schemes: `operator`, `cli`, `app`, `user`, `hub`, or `gateway`. |
| INV-PROT-ID-02 | The centralized ensemble broker identity MUST evaluate to the exact constant `spiffe://g8e.local/app/g8ee` (`protocol.EnsembleAppID`) and be validated by `protocol.WorkloadIdentity.IsEnsembleApp`. |
| INV-PROT-ID-03 | SPIFFE parsing and extraction methods (`ExtractCLISessionID`, `ExtractUserID`, `ExtractUserIDFromUserSAN`, `ExtractOperatorSessionID`, `ExtractGatewayID`) MUST fail closed and return `("", false)` if prefix matching, segment counting, or non-empty validation fails. |

### Canonicalization, Receipts, and Vectors (`INV-PROT-CANON`)

| ID | Rule |
| --- | --- |
| INV-PROT-CANON-01 | Canonical serialization of `ActionReceipt`, `ReceiptPersistenceAttestation`, compliance records, and evaluation reports MUST yield byte-identical UTF-8 JSON representations across Go and Python implementations. |
| INV-PROT-CANON-02 | Ed25519 signature verification across Go (`internal/services/governance`) and Python (`g8e.receipts`) MUST verify against identical canonical payload bytes using standard Ed25519 public keys in raw 32-byte or PEM SPKI formats. |
| INV-PROT-CANON-03 | Cross-language test vectors in `protocol/vectors/` and conformance suites in `protocol/conformance/` MUST pass cleanly in CI on every push and pull request before any release tag is cut. |

## Owned surfaces

| Claim | Path | Verify |
| --- | --- | --- |
| Go protocol package | `protocol/go_package.go`, `protocol/workload_identity.go` | `make -C protocol test` |
| Workload identity helpers | `protocol/workload_identity.go` | `go test -v ./protocol -run TestWorkloadIdentity` |
| Protobuf schemas and specs | `protocol/proto/g8e/`, `buf.gen.yaml` | `make proto && git diff --exit-code` |
| Python protocol package | `protocol/python/` | `pytest protocol/python/tests -v` |
| Python constants & enums | `protocol/python/g8e/constants.py`, `protocol/python/g8e/enums.py` | `pytest protocol/python/tests/test_constants.py -v` |
| TypeScript protobuf package | `protocol/node/` | `npm --prefix protocol/node run typecheck` |
| Observe contract generator | `dashboard/g8e-adapter/generator/gen-contract-pack.mjs` | `node dashboard/g8e-adapter/generator/gen-contract-pack.mjs --check` |
| Cross-language test vectors | `protocol/vectors/` | `go test -v ./protocol` |
| Conformance test suite | `protocol/conformance/` | `uv run --project protocol/python --extra dev pytest protocol/conformance -v` |
| JSON constant registries | `protocol/constants/` | `make validate-doctrines && make validate-cosais` |
| JSON model schemas | `protocol/models/` | `pytest protocol/conformance/test_models.py -v` |
| Release orchestration | `Makefile`, `.github/workflows/` | `make release` check steps |

## Procedures

### Multi-language protocol architecture and module structure

The g8e Protocol Library provides a unified wire contract across three language ecosystems:

1. **Go Protocol Package**: Located in the root module `github.com/g8e-ai/g8e/v2`. Import paths use `github.com/g8e-ai/g8e/v2/protocol/...`. The Go package delivers compiled protobuf structs, gRPC client/server interfaces, SPIFFE workload identity generators, and canonical receipt/attestation verification logic.
2. **Python Protocol Package**: Located in `protocol/python/` and published to PyPI as `g8e`. It delivers compiled protobuf stubs, runtime constant dictionaries, dynamic enum generators, Pydantic v2 validation models, and receipt signature verification.
3. **TypeScript Protobuf Package**: Located in `protocol/node/` under the private package name `@g8e/protocol-node`. Built using `@bufbuild/protobuf` and `@bufbuild/protoc-gen-es`, it provides typed protobuf message bindings for node consumers without being published to npm.

All surfaces share the version recorded in `VERSION`. Go is versioned through root git tags (`vX.Y.Z`), while Python is published through dedicated tags (`protocol/vX.Y.Z`).

### Go protocol package and workload identity helpers

The Go protocol package requires Go 1.26.6 or later and relies directly on `google.golang.org/grpc v1.84.0` and `google.golang.org/protobuf v1.36.12`.

SPIFFE workload identity generation and verification are implemented in `protocol/workload_identity.go` for the `g8e.local` trust domain (`protocol.TrustDomain`). Six workload identities are supported:

- **Operator**: Identifies target execution node instances: `spiffe://g8e.local/operator/<organization_id>/<operator_id>/<operator_session_id>`. Formatted via `OperatorSPIFFEID` and parsed via `ExtractOperatorSessionID`.
- **CLI**: Identifies authenticated command-line client sessions: `spiffe://g8e.local/cli/<user_id>/<cli_session_id>`. Formatted via `CLISPIFFEID` and parsed via `ExtractCLISessionID` and `ExtractUserID`. Initial session routing uses `MatchesCLISessionOnly`.
- **App**: Identifies external application and agent integrations: `spiffe://g8e.local/app/<operator_id>`. Evaluated via `AppSPIFFEID`, `MatchesApp`, and `IsAppSAN`.
- **Ensemble (g8ee)**: The centralized event broker that fans out SSE events to browser and CLI sessions: `spiffe://g8e.local/app/g8ee` (`protocol.EnsembleAppID`). Verified via `IsEnsembleApp`.
- **User**: Identifies human delegator sessions: `spiffe://g8e.local/user/<user_id>`. Formatted via `UserSPIFFEID`, checked via `IsUserSAN`, and parsed via `ExtractUserIDFromUserSAN`.
- **Hub**: Identifies central gateway listener endpoints: `spiffe://g8e.local/hub/operator-listen`. Formatted via `HubSPIFFEID` and validated via `MatchesHub`.
- **GatewayPeer**: Identifies peer gateway nodes in distributed deployments: `spiffe://g8e.local/gateway/<gateway_id>`. Formatted via `GatewayPeerSPIFFEID` and parsed via `ExtractGatewayID`.

Testing and quality verification within `protocol/` use standard make targets:

```bash
# Run unit and canonicalization tests
make -C protocol test

# Format code
make -C protocol fmt

# Static analysis and linting
make -C protocol vet
make -C protocol lint
```

### Python protocol package, constants loader, dynamic enums, and models

The Python package `g8e` requires Python 3.10 or later (tested across 3.10, 3.11, 3.12, 3.13, and 3.14). Runtime dependencies are `pydantic>=2.0.0`, `protobuf>=4.0.0`, and `PyNaCl>=1.5.0`.

#### Fail-closed constants loading

The constants loader (`g8e.constants._get_protocol_dir`) resolves the protocol definitions directory using a strict two-step order:

1. `G8E_PROTOCOL_DIR` environment variable: If set to a non-empty string, resolves to `Path(G8E_PROTOCOL_DIR) / "constants"`. If empty, it is treated as unset.
2. Bundled package data: Uses `g8e/_data/` packaged with pip installations.

If any required JSON constant file is missing, empty, or malformed, the loader fails closed by raising `ProtocolConstantsError` at import time. Exported dictionaries include `EVENTS`, `STATUS`, `MSG`, `COLLECTIONS`, `KV`, `CHANNELS`, `PUBSUB`, `INTENTS`, `PROMPTS`, `TIMESTAMP`, `HEADERS`, `DOCUMENT_IDS`, `PLATFORM`, `AGENTS`, `NETWORK`, `API_PATHS`, and `PORTS`. Typed lookups are provided by `collection()`, `channel()`, `document_id()`, `intent()`, `prompt()`, `kv_key()`, and `kv_session_type()`.

#### Dynamic enum generation

Module `g8e.enums` dynamically builds Python `StrEnum` and `IntEnum` classes from constant dictionaries:

- Enum member names follow `SCREAMING_SNAKE_CASE` (derived from `_python_const` or PascalCase event keys).
- Enum values preserve wire strings and integer codes verbatim.
- Integer categories (`citation_layout`, `priority`, `scrubber_priority`, `severity`, `slash_tier`) generate `IntEnum`; text categories generate `StrEnum`.
- Exports `EventType` generated from `EVENTS`, all status categories from `STATUS`, and non-STATUS categories (`Channel`, `Intent`, `Prompt`, `Collection`, `KVKey`).
- Enums are constructed lazily on demand and cached in memory using `lru_cache`.

#### Event registry lookup

Module `g8e.registry` provides cached lookups over `EVENTS["events"]`:

- `action_for(event_type)`: Returns the governed action class for a request event, verifying that `kind == "request"`, `transport` contains `"governed"`, and `governance.action_type` is populated; fails closed with `UnknownEventError` or `EventNotGovernedError`.
- `meta(event_type)`: Retrieves raw registry metadata for an event.

#### Pydantic models

Models in `g8e.models` extend `G8eBaseModel` (`populate_by_name=True`, `extra="ignore"`, serializing UTC datetimes with a `Z` suffix):

- **Context (`g8e.models.context`)**: `RequestContext`, `BoundOperator`.
- **Internal API (`g8e.models.internal_api`)**: `ChatMessageRequest`, `ChatStartedResponse`, `ResourceCreationRequest`, `LLMOverrides`.
- **Events (`g8e.models.events`)**: `SessionEventWire`, `BackgroundEventWire`, `AiProcessingStoppedPayload`, `AIToolLifecyclePayload`, `ChatCitationsReadyPayload`, `ChatErrorPayload`, `ChatProcessingStartedPayload`, `ChatResponseChunkPayload`, `ChatResponseCompletePayload`, `ChatRetryPayload`, `ChatThinkingPayload`, `ChatTurnCompletePayload`, `TriageClarificationQuestionsPayload`, `AgentStatusUpdatedPayload`, `RunStatusUpdatedPayload`, `EvalRunCompletedPayload`, `EvalMetricRecordedPayload`, `ObservedMeasurement`.
- **Observe API (`g8e.models.observe_api`)**: Read models (`ObserveBootstrapSnapshot`, `RunDetail`, `EvalDetail`, `DownloadArtifact`) and mTLS producer models configured with `extra="forbid"` (`ObserveProducerAgentStateRequest`, `ObserveProducerRunStateRequest`, `ObserveProducerResponse`).
- **Public Feed (`g8e.models.public_feed`)**: Outbound public-spectator batches, snapshots, proofs, and ingestion requests (`PublicFeedBatch`, `PublicFeedSnapshot`, `PublicFeedBootstrap`, `PublicIngestRequest`, `PublicProofManifest`).
- **Governance Envelope (`g8e.models.governance`)**: `GovernanceEnvelope`, `GovernanceMetadata`, `GovernanceL1`, `GovernanceL2`, `GovernanceL2Vote`, `GovernanceL3`, `GovernanceL3Proof`, `CommandIntent`, `compute_transaction_hash`.
- **Settings (`g8e.models.settings`)**: `PlatformSettings`, `G8eeUserSettings`, `LLMSettings`, `SearchSettings`, `EvalJudgeSettings`, `CommandValidationSettings`, `BatchExecutionSettings`.

### TypeScript generated protobuf package and observe contract generator

#### TypeScript protobuf bindings

`protocol/node/` contains `@g8e/protocol-node`, a private module delivering TypeScript protobuf bindings generated with `@bufbuild/protoc-gen-es` (v2.14.0) and `@bufbuild/protobuf` (v2.14.0). The package covers the `common`, `compliance`, `eval`, `operator`, and `pubsub` packages.

To regenerate and typecheck:

```bash
# Generate TypeScript protobuf bindings
make proto-node

# Verify compilation under strict NodeNext settings
npm --prefix protocol/node run typecheck
```

#### Observe frontend contract pack generator

The deterministic generator at `dashboard/g8e-adapter/generator/gen-contract-pack.mjs` consumes canonical protocol JSON (`protocol/models/observe_api.json`, `protocol/models/observe_event_payloads.json`, `protocol/constants/event_dashboard_classification.json`) and the compiled adapter distribution to produce `contract-pack/models.ts`. This contract pack provides TypeScript models, runtime validators, type guards, and fixtures for browser-facing read projections and event payloads. Internal mTLS producer models are excluded. Re-running against identical inputs produces byte-identical files, verified in CI using `node dashboard/g8e-adapter/generator/gen-contract-pack.mjs --check`.

### Shared protocol assets: constants registries, model schemas, and MCP configurations

#### Constants registries

The directory `protocol/constants/` maintains 26 top-level JSON registries alongside compliance and doctrine catalogs:

- Registries cover events (`events.json`), status codes (`status.json`), collections (`collections.json`), API paths (`api_paths.json`), auth parameters (`auth.json`), headers (`headers.json`), KV keys (`kv_keys.json`), channels (`channels.json`), pubsub topics (`pubsub.json`), intents (`intents.json`), prompt templates (`prompts.json`), agent roles (`agents.json`), platform parameters (`platform.json`), enrollment parameters and vectors (`platform_enrollment.json`, `platform_enrollment_completion_transcript_vectors.json`), exit codes (`exit_codes.json`), field paths (`field_paths.json`), document types (`document_ids.json`), network parameters (`network.json`), output formats (`output.json`), default ports (`ports.json`), timestamp formats (`timestamp.json`), and environment variables (`env_vars.json`).
- `protocol/constants/compliance/`: Catalogs for assertions (`assertion-catalog.json`), frameworks (`framework-catalog.json`), FedRAMP/NIST crosswalks (`fedramp-nist-crosswalk.json`), and demo scenarios (`demo-scenario-catalog.json`).
- `protocol/constants/doctrine/`: L1 threat detection pattern registries defining blacklist rules (`blacklist_doctrine.json`), whitelist rules (`whitelist_doctrine.json`), Gitleaks secrets signatures (`gitleaks_doctrine.json`), OWASP Core Rule Set patterns (`owasp_crs_doctrine.json`), and MCP attack vectors (`mcp_vectors_doctrine.json`).
- `protocol/constants/event_dashboard_classification.json`: Enforces frontend observability relationships: `produced_to_sse`, `governed_record_only`, `mixed`, or `unsupported`.

#### JSON model schemas

`protocol/models/` contains 56 platform JSON Schema definitions for core structures including envelopes, approvals, cases, chat messages, CLI sessions, consensus configurations, operator documents, observe API read models, passkey credentials, and tool results. Per-agent role schemas are maintained in `protocol/models/agents/` (`assistant.json`, `auditor.json`, `lite.json`, `primary.json`, `title_generator.json`, `triage.json`). Third-party validated schemas reside under `protocol/schemas/`, including NIST OSCAL 1.1.2.

#### MCP server deployment configurations

Example deployment configurations in `examples/mcp-client-configs/` illustrate governed MCP topologies:

- `g8e_gateway_mcp_config.json`: Production HTTP with mTLS using client certificate paths.
- `g8e_stdio_mcp_config.json`: Local development stdio mode executing `g8e mcp stdio`.
- `g8e_agent_mcp_config.json`: Agent governance configuration routing tool executions through governed gateway endpoints under the agent's application identity (`--app <agent>`) while excluding raw execution primitives (`Bash`, `Read`, `Write`, `Edit`, `Glob`, `Grep`, `WebSearch`, `WebFetch`).

### Protobuf compilation and code generation workflow

Protobuf compilation is managed via Buf using configurations in `protocol/proto/buf.yaml` (module `buf.build/g8e/platform`) and root `buf.gen.yaml`.

```bash
# Generate all protocol bindings (Go, Python, Node, and lockfiles)
make proto
```

The `make proto` pipeline executes four coordinated targets:

1. `make proto-go`: Installs Buf if absent and executes `buf generate protocol/proto` using `protoc-gen-go` (v1.36.11), `protoc-gen-go-grpc` (v1.6.2), and `protoc-gen-doc` (v1.5.1), producing Go code and API reference docs in `protocol/docs/reference/api`.
2. `make proto-python`: Executes `python protocol/python/scripts/generate_protos.py` using `grpc_tools.protoc` to generate Python modules and `.pyi` type stubs.
3. `make proto-node`: Executes `npm ci --prefix protocol/node` and runs `buf generate` using `@bufbuild/protoc-gen-es` to emit TypeScript stubs into `protocol/node/src/gen/`.
4. `make proto-lockfiles`: Synchronizes downstream lockfiles (`ensemble/uv.lock`) that depend on `protocol/python`.

### Canonical serialization, receipt verification, and cross-language vectors

#### Deterministic stage evidence and receipt canonicalization

Governed execution yields an `ActionReceipt` carrying five-layer execution evidence, state Merkle roots, failure codes, and execution results.

- In Go, `internal/services/governance.CanonicalizeActionReceipt` formats the receipt into canonical UTF-8 JSON. Deterministic stage evidence is hashed by sorting and serializing protobuf stages deterministically with length-prefix encoding before SHA-256 digesting.
- In Python, `g8e.receipts.canonicalize_action_receipt` mirrors this exact structure and hashing logic.
- Both surfaces verify Ed25519 signatures using raw 32-byte public keys or PEM SPKI keys (`verify_action_receipt`, `verify_persistence_attestation`).

#### Cross-language vectors

Evaluation protobuf JSON uses `evalv1.MarshalCanonical` and `evalv1.UnmarshalCanonical` in `protocol/proto/g8e/eval/v1/canonical.go`. The encoder uses protobuf field names and omits proto3 scalar defaults. `protocol/vectors/eval/public_assignment_result_failed.json` contains Go-produced bytes for a completed assignment with verdict `FAIL` and a zero `task_score` whose `value` key is omitted. `TestFailedPublicAssignmentResultCanonicalizationMatchesCrossLanguageVector` constructs the producer message and checks byte equality; the Explorer's `tests/campaign-adapter.test.ts` decodes the same bytes through `decodeCampaignProjectionEnvelope` and checks `task_score = 0`, `pass = 0`, and `model_failed`. The decoder applies the proto3 default only to a present score record; a missing score record does not imply zero.

Live events are the protobuf message `PublicLiveEvent`, encoded only by `MarshalPublicLiveEvent` through `evalv1.MarshalCanonical`. Its `completed`, `total`, and `PublicLiveMetricValue.value` fields are `optional`, so a zero is emitted rather than omitted, and a producer or browser that finds either count absent rejects the event. `protocol/vectors/eval/public_live_event.json` contains Go-produced bytes for a `metric_updated` event with no progress and a measured `pass` of 0. `TestPublicLiveEventCanonicalizationMatchesCrossLanguageVector` constructs the producer message and checks byte equality; the Explorer's `tests/live-event-wire.test.ts` decodes the same bytes through `decodeLiveEventWire`.

`protocol/vectors/` contains cross-language verification vectors ensuring bit-for-bit parity:

- `action_receipt_canonicalization.json`: Canonical UTF-8 JSON encoding and Ed25519 verification vectors.
- `action_receipt_failure_code_canonicalization.json`: Standardized error code canonicalization.
- `action_receipt_stage_evidence_canonicalization.json`: Deterministic stage evidence hashing vectors.
- `receipt_persistence_attestation_canonicalization.json`: Storage persistence attestation vectors.
- `vectors/compliance/`: Control assertion definitions and compliance record canonicalization vectors.
- `vectors/eval/`: Evaluation reports, model campaign specifications, and assignment result vectors.

Go tests in `protocol/*_canonicalization_test.go` and Python tests in `protocol/python/tests/` evaluate against these identical vector files.

### Cross-language conformance testing suite

The conformance suite in `protocol/conformance/` verifies continuous parity between Go, Python, and JSON schema definitions:

- `test_constants.py`: Validates JSON constant registry structure, checks that every entry contains `_go_const` and `_python_const` metadata, and confirms value uniqueness.
- `test_models.py`: Validates structural field matching, enum values, and validation behavior between Pydantic models in `g8e.models` and JSON Schema definitions in `protocol/models/`.
- `test_hash_parity.py`: Validates SHA-256 transaction hash parity between Python `compute_transaction_hash` and Go envelope hashing using `hash_vectors.json` and `hash_vectors_v2.json`.

Run the full conformance suite:

```bash
uv run --project protocol/python --extra dev pytest protocol/conformance -v
```

### Unified versioning, release workflow, and CI pipelines

#### Version sync validation

All protocol artifacts adhere to the semantic version in `VERSION` (`v2.2.6`). CI job `Verify Version Sync` in `.github/workflows/build-and-test.yml` strictly validates:

1. `protocol/python/pyproject.toml` (`version = "X.Y.Z"`)
2. `protocol/python/g8e/__init__.py` (`__version__ = "X.Y.Z"`)
3. `protocol/python/uv.lock` (`name = "g8e"` package version)
4. `protocol/docs/a2a.md` (`Version: vX.Y.Z`)
5. `protocol/docs/constants.md` (`Version: vX.Y.Z`)
6. `protocol/docs/mcp.md` (`Version: vX.Y.Z`)
7. `protocol/docs/spec.md` (`Version: vX.Y.Z`)

#### Release execution

Releases are executed through `make release`:

```bash
make release
```

The target executes the following steps:
1. Strips any leading `v` from `VERSION` and reads the target version.
2. Synchronizes version strings across `pyproject.toml`, `__init__.py`, `uv.lock`, and protocol documentation headers.
3. Checks `git status --porcelain`; if any files were changed by the sync, it aborts immediately and requires the maintainer to commit the changes first.
4. Verifies the existence of release notes at `docs/release_notes/v<MAJOR_MINOR>.x/v<VERSION>.md`.
5. Confirms that git tags `v<VERSION>` and `protocol/v<VERSION>` do not already exist on remote.
6. Cuts both tags simultaneously on the HEAD commit and pushes them to `origin`.

#### CI release automation

- **Platform Binary Pipeline (`release-binary.yml`)**: Triggered by `v*` tags. Builds cross-platform binaries using Go 1.26.6, generates SHA-256 checksums, signs assets with Cosign, and publishes GitHub releases.
- **Python Protocol Pipeline (`release-python-protocol.yml`)**: Triggered by `protocol/v*` tags. Validates metadata with Python 3.14, bundles JSON constant registries into `protocol/python/g8e/_data/`, builds source distribution and wheels, publishes to PyPI via trusted publishing, and verifies cross-platform pip installation by polling the PyPI JSON API across Ubuntu, macOS, and Windows runners.

## Anti-patterns

- Hand-editing generated protobuf code or Swagger files instead of editing `.proto` schemas and running `make proto` (INV-PROT-SCHEMA-01).
- Bumping `VERSION` or document headers without running the full audit workflow, synchronizing Python metadata, and verifying test vectors (INV-PROT-VER-01).
- Introducing unvalidated directory probes or falling back to checkout source trees in the Python constants loader (INV-PROT-CONST-02).
- Emitting raw binary protobuf over public or browser ingress endpoints instead of canonical protojson (INV-PROT-SCHEMA-02).
- Attempting to use `protocol/v*` tags for Go module resolution; Go modules are versioned strictly through root `v*` tags (INV-PROT-VER-02).
- Hard-wrapping source prose lines or embedding source code line numbers in documentation (INV-DOC-STYLE-01, INV-DOC-STYLE-02).
- Manually editing TypeScript contract models instead of updating protocol JSON and executing `gen-contract-pack.mjs`.

## Links out

- [Governance Architecture](governance.md): Five-layer governance verification pipeline (L1-L5), postures, and envelope structures.
- [Consensus Architecture](consensus.md): L2 multi-member Ed25519 signature policies and deliberation.
- [Network Architecture](network.md): Platform PKI, TLS/mTLS, SPIFFE identity SAN issuance, and port topology.
- [Gateway Architecture](gateway.md): Gateway ingress routing, policy decision points, and dispatch mechanics.
- [Operator Architecture](operator.md): Operator runtime daemon, local execution, and L4/L5 verification.
- [Event and Action Protocol](events.md): Event catalog taxonomy, SSE streaming, and payload structures.
- [AI Agents and Boundary](agents.md): Agent roles, downstream MCP egress, and tool boundaries.
- [Release Process](../devs/release_process.md): Detailed release workflows, compliance bundles, and native eval acceptance.
- [Documentation Guide](../devs/docs.md): Standards, invariants, and procedures for g8e documentation.
- [Protocol Specification](../../protocol/docs/spec.md): Canonical envelope structure and 5-layer interlock sequence details.
- [Constants Reference](../../protocol/docs/constants.md): Platform constants system reference.
- [A2A Protocol](../../protocol/docs/a2a.md): Agent-to-Agent protocol integration specification.
- [MCP Protocol](../../protocol/docs/mcp.md): Model Context Protocol integration specification.
- [Observe Frontend Builder Guide](../guides/build_observe_frontend.md): Contract pack generator and builder workflows.
