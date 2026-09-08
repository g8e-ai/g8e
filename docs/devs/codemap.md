# g8e Code Map

This document maps the current repository by runtime entry point, service boundary, and supporting component. It identifies where behavior is owned without duplicating protocol specifications or user guides. The codebase remains the source of truth.

## Repository Map

- `cmd/g8e/`: Go binary entry point. `main.go` passes build metadata to the Cobra command package.
- `internal/cli/`: CLI commands, configuration loading, enrollment, process management, service startup, SSE clients, streaming, the terminal UI, and the onboarding wizard.
- `internal/services/`: Gateway, Operator, governance, transport, persistence, execution, compliance, and supporting services.
- `internal/adapters/`: Optional external system adapters. The current adapter integrates an Operator with Anduril Lattice.
- `internal/constants/`: Go constants for paths, errors, protocol identifiers, permissions, and runtime behavior.
- `internal/models/`: Internal typed models used by services and CLI code.
- `internal/paths/`: Initialized runtime paths derived from the configured project root.
- `internal/certs/`, `internal/httpclient/`, `internal/marshaler/`, `internal/response/`, and `internal/security/`: Shared certificate, HTTP, serialization, response, and security infrastructure.
- `protocol/`: Canonical protobuf schemas, JSON registries, JSON model schemas, generated language bindings, conformance tests, vectors, examples, and protocol documentation.
- `test/`: Cross-package integration tests, reusable gateway fixtures, and Docker E2E tests.
- `ensemble/`: Python g8ee application, agent ensemble, evaluation harness, and tests. See [Ensemble documentation](../ensemble/index.md).
- `dashboard/`: Node.js g8ed static SPA host and browser application. See [Dashboard documentation](../dashboard/index.md).
- `demos/`: Healthcare, finance, DHS, and FedRAMP demo environments.
- `docs/`: Architecture, guides, references, developer documentation, release notes, and generated README inputs.
- `scripts/`: Validation, generation, release, and build support scripts.
- `website/`: Static website generator and Cloudflare Worker packaging.
- `third_party/`: Vendored source inputs that are generated into internal adapters.
- `Makefile`, `Dockerfile`, and `docker-compose.yml`: Repository-wide build, validation, image, and deployment orchestration.

## Runtime Entry Points

The executable starts in `cmd/g8e/main.go`. Command registration and process-level error handling live in `internal/cli/cmd/main.go`.

The root command registers these command groups:

- `gw`: Gateway lifecycle, setup, data administration, security validation, and tunnel management.
- `auth`: User and frontend enrollment, session management, approvals, recovery, platform enrollment, and automation context output.
- `mcp`: MCP stdio serving and supported agent integration.
- `operator`: Operator discovery, startup, deployment, file transfer, and stream management.
- `vault`: Local vault initialization, unlock, rekey, status, reset, export, and import.
- `test`: Unit, integration, E2E, coverage, lint, chaos, and summary workflows.
- `demos`: Demo environment and scenario lifecycle.
- `docker`: Unified Docker Compose stack lifecycle.
- `audit`: Receipt, event, summary, export, and report queries.
- `report`: Deterministic CSV evidence generation and offline verification.
- `compliance`: KSI evaluation, KSI history, overlay validation, demo-run verification, release evidence, evidence graph verification, and signed compliance report workflows.
- `swagger`: OpenAPI generation, serving, and validation.
- `tui`: Tactical Governance Console.
- `version`: Build metadata and optional FIPS module status.

Use `g8e <command> --help` for the live command and flag hierarchy. The command constructors in `internal/cli/cmd/` are the implementation source of truth.

### Service Startup

- `internal/cli/serve/gateway.go`: Initializes paths and `RuntimeFileService`, creates the runtime tree, configures logging, loads gateway configuration, constructs `GatewayModeService`, exports the Actuator public key, starts the gateway and its in-process command service, and coordinates shutdown.
- `internal/cli/serve/operator.go`: Loads Operator configuration and credentials, performs platform enrollment when needed, builds mTLS transport, starts `G8eoService`, runs certificate renewal, and coordinates shutdown.
- `internal/cli/serve/platform_enrollment_client.go`: Implements the resumable owner-approved Operator enrollment protocol.
- `internal/cli/serve/cert.go`: Owns Operator certificate loading, renewal, trust bundle retrieval, and mTLS client construction.
- `internal/services/logging/`: Owns daemon log file creation and structured logger configuration.

## Runtime Modes

### Gateway Mode

`gateway.GatewayModeService` in `internal/services/gateway/gateway_service.go` is the top-level gateway runtime. Its builder assembles dependencies before the service starts.

The gateway runtime owns these major groups:

- Persistence: `CanonicalDBService` and its document, app policy, signer, consensus, state root, replay, KV, SSE, blob, and SQL audit stores.
- Identity and authorization: `PKIAuthority`, `AuthService`, `RegistrationService`, persona and user services, CLI, Operator, and web session services, enrollment tokens, CLI recovery, passkeys, and platform enrollment.
- Transport: two HTTP servers, controller-based routing, the in-process WebSocket pub/sub broker, SSE, MCP, and A2A ingress.
- Governance: L1 doctrine, gateway L3 notary, optional L2 consensus, `OperatorPubSubService`, L4 verification, L5 actuation, and signed audit output.
- Execution: in-process execution and file-edit services used after governance verification.

The builder creates the command service before the MCP gateway, injects the command service into the MCP gateway as the envelope processor and session validator, then binds the MCP gateway back to the command service once before startup. `PlatformEnrollmentService` also routes mutations through the command service as a governance envelope processor.

### Outbound Operator Mode

`services.G8eoService` in `internal/services/g8eo.go` is the top-level outbound Operator runtime. It initiates authenticated connections to the gateway and executes approved work on the Operator host.

The outbound runtime owns these major groups:

- Bootstrap and transport: `auth.BootstrapService`, an mTLS pub/sub client, `PubSubResultsService`, and `OperatorPubSubService`.
- Execution: `ExecutionService` and `FileEditService`.
- Persistence: `CanonicalDBService`, the shared encrypted vault, execution vault, encrypted KV token adapter, suspended transaction store, replay store, SQL audit store, and optional Git ledger history.
- Governance: local L1 doctrine, filesystem signer trust, outbound L3 notary, remote gateway state root verification when connected, L4 verification, and L5 actuation.
- External integration: the optional Lattice adapter receives tasks and publishes Operator presence through the same governed execution path.

`pubsub.GovernanceCoreDeps` contains dependencies shared by both modes. `pubsub.GatewayModeDeps` adds governed document storage, consensus, field reads, platform enrollment, and posture, while `pubsub.OutboundModeDeps` exposes only the shared governance dependencies.

## Governance and Execution Flow

All mutations enter the governance pipeline as a typed `GovernanceEnvelope`. The canonical envelope and proof messages live in `protocol/proto/g8e/common/v1/`, while event and payload messages live in the Operator and pub/sub protobuf domains.

The verification sequence is:

1. **L1 Doctrine** scans the requested action for hard policy violations, forbidden patterns, and recognized threat signals. The implementation lives in `internal/services/governance/l1_doctrine.go`.
2. **L2 Consensus** verifies policy membership, Ed25519 votes, vetoes, and quorum when the active posture requires consensus. Consensus construction and deliberation live in `internal/services/consensus/`.
3. **L3 Notary** verifies human authorization when the posture requires it. Gateway mode composes passkey and CLI session verification; outbound mode verifies approval stored with a suspended transaction.
4. **L4 Warden** verifies the complete pre-dispatch transaction, including signatures, replay protection, expiry, nonce, state Merkle root, and required L1, L2, and L3 evidence. It emits a `VerifiedTransaction`, not an executable request.
5. **L5 Actuator** dispatches only verified work to MCP, A2A, command, or file execution handlers, then records signed receipts and audit evidence.

`internal/services/pubsub/` coordinates command and file-operation handlers with L4 and L5. `internal/services/mcp/` translates MCP and A2A requests, registers native tools, handles downstream dispatch, scrubs governed data, and participates in suspension and resumption. `internal/services/scrubbing/` tokenizes sensitive values through the configured token store before data crosses an execution boundary.

## Gateway HTTP Boundary

`gateway.HTTPHandler` is a routing and middleware shell. Domain handlers are split into controllers under `internal/services/gateway/` for PKI, audit, data, signers, bootstrap, CLI recovery and rotation, enrollment tokens, users and sessions, administration, Operators, dispatch, SSE, health, governance, MCP, pub/sub, passkeys, and platform enrollment.

The gateway exposes two router surfaces:

- The HTTPS router serves the full API, applies route-specific mTLS, web-session, dual-auth, or JWT handling, and defaults unknown protected routes to mTLS.
- The HTTP router serves limited bootstrap and discovery operations, including trust material and public enrollment workflows, and redirects other traffic to HTTPS.

Cross-cutting middleware owns authentication classification, privileged-route restrictions, enrolled-origin CORS, rate limiting, and path traversal rejection. Embedded Console assets, deployment scripts, and generated OpenAPI files live under `internal/services/gateway/console/`, `internal/services/gateway/scripts/`, and `internal/services/gateway/docs/`.

## Persistence and Runtime Files

### Canonical Gateway Database

`CanonicalDBService` in `internal/services/gateway/gateway_db.go` owns the primary SQLite connection, schema lifecycle, encrypted vault, secret manager, maintenance loop, and store lifetimes. Consumers receive narrow store services through typed accessors rather than raw database access.

The principal stores are:

- `DocumentStoreService`: collection and document persistence, governed document mutation, and field reads.
- `AppPolicyStoreService`: application policy lookup.
- `SignerStoreService`: trusted governance signer persistence.
- `ConsensusStoreService`: consensus policy persistence.
- `StateRootService`: cached state Merkle root calculation.
- `ReplayStoreService`: gateway nonce replay prevention.
- `KVStoreService`: TTL-aware key-value state.
- `SSEEventService`: durable SSE event storage and fan-out support.
- `BlobStoreService`: binary object persistence.
- `storage.SQLAuditStore`: audit events, sessions, commitments, and signed receipts.

Additional stores in `internal/services/storage/` cover execution vault records, suspended transactions, standalone replay protection, Git-backed file history, commitments, and reporting inputs.

### Runtime File Service

`RuntimeFileService` in `internal/services/fs/file_service.go` is the canonical abstraction for `.g8e/` file I/O. Startup calls `CreateRuntimeTree`, and services use relative paths built from `internal/constants/paths.go` with `ReadFile`, `WriteFile`, `Stat`, `ReadDir`, `Rename`, `Remove`, streaming open methods, and permission enforcement.

Use `Resolve` only when an API requires an absolute path and `Rel` when converting an absolute runtime path back to the service boundary. Tests inspect runtime files through `fileSvc.ReadFile` or `fileSvc.Stat`, check existence with `fileSvc.FileExists`, and compare missing-file errors with `errors.Is(err, constants.ErrNotFound)`.

## Internal Service Packages

- `internal/services/auth/`: Operator bootstrap transport and system fingerprinting.
- `internal/services/compliance/`: KSI models and evaluation, history and unavailable intervals, OSCAL support, catalog validation, evidence import and graph verification, assertion grading, and signed report bundles.
- `internal/services/consensus/`: Consensus members, policy-based service construction, deliberation, and Ed25519 voting.
- `internal/services/execution/`: Command execution and governed file edits.
- `internal/services/fs/`: Scoped `.g8e/` runtime file operations.
- `internal/services/gateway/`: Gateway orchestration, HTTP controllers, identity, PKI, enrollment, persistence stores, pub/sub, and embedded assets.
- `internal/services/governance/`: L1, L3, L4, L5, governance interfaces, state root providers, signer stores, and public-key export.
- `internal/services/keystore/`: Encrypted key storage used by gateway secrets and PKI.
- `internal/services/logging/`: Runtime log file and `slog` configuration.
- `internal/services/mcp/`: MCP and A2A gateway, native tools, field-path governance, suspension, and downstream clients.
- `internal/services/network/`: Network identity detection and endpoint construction.
- `internal/services/pubsub/`: Gateway and Operator pub/sub clients, command dispatch, results, heartbeats, ports, audit, history, and governance mode wiring.
- `internal/services/reporting/`: Deterministic CSV evidence reports and cryptographic verification.
- `internal/services/scrubbing/`: Sensitive-value detection, tokenization, and rehydration.
- `internal/services/sqliteutil/`: Shared SQLite configuration and connection helpers.
- `internal/services/storage/`: Audit, execution, replay, suspension, commitment, and Git ledger persistence.
- `internal/services/system/`: Host capability and embedded Git selection.
- `internal/services/vault/`: Encryption vault lifecycle and cryptographic storage.

## CLI Packages

- `internal/cli/cmd/`: Cobra command tree and dependency-injected command constructors.
- `internal/cli/auth/`: CLI enrollment coordinator, gateway enrollment transport, credential staging, key generation, passkey registration, trust bundle loading, and mTLS clients.
- `internal/cli/api/`: Typed CLI HTTP client.
- `internal/cli/config/`: CLI-facing configuration resolution and endpoint overrides.
- `internal/cli/serve/`: Gateway and Operator foreground runtimes, platform enrollment, certificate renewal, and build version metadata.
- `internal/cli/platform/`: Cross-platform process, browser, and system trust operations.
- `internal/cli/sse/`: CLI SSE client.
- `internal/cli/stream/`: Local and SSH stream handling for Operator management.
- `internal/cli/tui/`: Tactical Governance Console.
- `internal/cli/wizard/`: Interactive gateway setup flow.

Command functions that access `.g8e/` receive a `fileSvcFactory`. Their factory initialization errors wrap `constants.ErrFileServiceInit`, and matching tests live in `internal/cli/cmd/factory_error_test.go`.

## Protocol and Generated Packages

`protocol/proto/g8e/` contains four protobuf domains:

- `common/v1`: Governance envelopes, layer metadata, shared enums, validation options, and common messages.
- `compliance/v1`: Compliance evidence, assessment, and report messages.
- `operator/v1`: Operator commands, execution results, telemetry, receipts, and service RPC definitions.
- `pubsub/v1`: Pub/sub event and message envelopes.

`buf.gen.yaml` generates Go packages beside the schemas, Python modules under `protocol/python/g8e/`, TypeScript modules under `protocol/node/src/gen/`, and Markdown API references under `protocol/docs/reference/api/`.

Other protocol surfaces are:

- `protocol/constants/`: External JSON references for wire identifiers, paths, events, statuses, channels, authentication, enrollment, and compliance catalogs. Go runtime constants in `internal/constants/` remain the implementation source of truth.
- `protocol/models/`: JSON schemas for client-facing model shapes.
- `protocol/schemas/`: Additional validation schemas.
- `protocol/vectors/` and `protocol/test-fixtures/`: Cross-language canonicalization vectors and fixtures.
- `protocol/conformance/`: Cross-language constants, model, and hash parity tests.
- `protocol/python/`: Published Python protocol package.
- `protocol/node/`: Private generated TypeScript protocol package.
- `protocol/examples/`: Go examples and MCP client configuration templates.
- `protocol/docs/`: Protocol specifications and generated API references.

## External Adapters and Native Tools

`internal/adapters/lattice/` integrates outbound Operators with Anduril Lattice over gRPC. It owns OAuth client credentials, retry and token refresh, presence publication, task streaming, governance handoff, and generated Lattice protobuf bindings.

MCP native tools are registered explicitly in `internal/services/mcp/native_tool_registry.go`. The registry covers controlled shell execution, filesystem inspection and mutation, database inspection, log analysis, process and host telemetry, network and TLS diagnostics, configuration inspection, container and Kubernetes inspection, cloud metadata, deployment, and audit receipt queries. Tool calls still pass through the active governance posture before execution.

## Compliance and Reporting

The offline CSV reporting path is implemented in `internal/services/reporting/` and exposed by `g8e report`. It reads audit, execution, replay, suspension, commitment, and Git ledger evidence, writes deterministic CSV files, and verifies signatures, commitment chains, receipt links, mutation links, and ledger roots.

The proof-backed compliance path is implemented in `internal/services/compliance/` and exposed by `g8e compliance`. It includes:

- Typed KSI catalogs, bound evaluations, historical snapshots, and unavailable intervals.
- Canonical assertion, framework, crosswalk, and demo scenario catalogs.
- Evidence importers for audit, receipts, commitments, ledgers, demos, evaluations, attestations, and build configuration.
- Scope-bound evidence graphs and verification reports.
- OSCAL validation and rendering support.
- Deterministic signed compliance report bundle generation and offline verification.
- Release evidence provenance collection.

Runtime compliance paths are centralized in `internal/constants/paths.go`; external path references live in `protocol/constants/compliance_paths.json`.

## Other Product Components

### Ensemble

`ensemble/app/main.py` is the g8ee application entry point. `ensemble/app/` contains API routers, middleware, typed models, LLM integrations, storage, security, gateway clients, and orchestration services. `ensemble/evals/` contains the standalone evaluation harness, and `ensemble/tests/` contains Python unit and integration tests.

### Dashboard

`dashboard/server.js` starts the g8ed Express application. The server resolves the dashboard workload identity before listening, serves the browser SPA and `g8e-config.js`, and injects the required browser-facing gateway origin. The browser calls the gateway directly with web-session credentials; the dashboard server does not proxy gateway API or WebSocket traffic.

Dashboard application code lives in `dashboard/public/`, workload enrollment lives in `dashboard/services/infra/`, and tests live in `dashboard/test/`.

### Demos and Agent Harness

`internal/tools/agent_harness/` is a typed reference client for submitting real governance envelopes, exercising MCP and A2A, waiting for human approval, and querying audit evidence. Scenario implementations cover governance postures and the healthcare, finance, DHS, and FedRAMP environments.

`demos/` contains the corresponding containerized services, datasets, actuator bridges, and verification scripts. Demo commands in `internal/cli/cmd/` orchestrate containers and persist typed compliance evidence through the same governed runtime surfaces.

### Website and README

The root `README.md` is generated from `docs/templates/README.md.tmpl` and the reviewed public evidence snapshot in `docs/evidence/readme/current/`. `scripts/generate_readme.py` validates the manifest and checksums before rendering. `website/` converts the README into the static site and packages the Cloudflare Worker. The [Documentation Guide](docs.md) catalogs every first-party documentation surface and defines audit, ownership, generation, cross-linking, metadata, and validation rules.

## Test Map

Tests follow three platform tiers:

- Tier 1 unit tests live beside Go packages and use stubs at external boundaries.
- Tier 2 integration tests use the `integration` build tag, real local SQLite and PKI infrastructure, and in-process gateway fixtures. Cross-package suites live in `test/`, with reusable setup in `test/fixtures/`.
- Tier 3 E2E tests use the `e2e` build tag and live in `test/e2e/`. They connect to an already running, approved platform over its external HTTP, mTLS, pub/sub, and component surfaces.

Test-only implementations live under packages such as `internal/services/storage/storagetest/`, `internal/services/pubsub/pubsubtest/`, and `internal/services/governance/governancetest/`. Production packages do not depend on them.

Run platform verification through the CLI:

- `./g8e test unit`
- `./g8e test integration`
- `./g8e test e2e`
- `./g8e test coverage`
- `./g8e test lint`

See [Testing Guide](./tests.md) for fixture conventions, build tags, and the full verification matrix.

## Build and Validation Map

The root `Makefile` coordinates protobuf generation, Go builds, Python protocol packaging, platform tests, component tests, linting, vulnerability checks, Swagger generation, doctrine and COSAiS validation, README generation, website generation, Docker builds, and FIPS builds.

The primary boundaries are:

- `make proto`: Regenerates Go, Python, TypeScript, and Markdown protobuf outputs.
- `make lint`: Runs Go lint and repository quality checks, including doctrine, COSAiS, vulnerability, and Swagger validation.
- `make test`, `make test-unit`, `make test-integration`, and `make test-docker`: Run the platform test tiers.
- `make python-build`: Builds the Python protocol distribution with bundled registries.
- `make dashboard-test`, `make ensemble-test`, and `make website-test`: Validate non-Go components.
- `make build-fips` and `make verify-fips`: Build and verify the pinned Linux AMD64 FIPS variant. See [FIPS 140-3 Compliance](../reference/fips140-3.md).
- `make readme`, `make readme-check`, and `make readme-test`: Generate and validate the public README and evidence projection.
