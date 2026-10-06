---
doc_id: docs
title: Documentation Guide
audience: maintainers and coding agents
status: current
last_updated: 2026-10-06
version: v2.3.1
owners:
  - docs/
  - protocol/docs/
  - README.md
  - buf.gen.yaml
  - Makefile
  - internal/services/gateway/docs/
  - internal/tools/constgen/
related:
  - docs/devs/devs.md
  - docs/devs/codemap.md
  - docs/devs/tests.md
  - docs/devs/release_process.md
  - docs/devs/troubleshooting.md
  - docs/devs/python-linting.md
when_to_read: Creating, editing, auditing, generating, or reviewing g8e documentation across handwritten, generated, and machine-readable surfaces.
do_not_use_for:
  - Platform coding invariants (docs/devs/devs.md)
  - Runtime and package ownership maps (docs/devs/codemap.md)
  - Test selection, fixtures, and CI scope (docs/devs/tests.md)
  - Release execution, native eval acceptance, and signed evidence (docs/devs/release_process.md)
  - Diagnostics, failure modes, and recovery (docs/devs/troubleshooting.md)
---

# Documentation Guide

## Purpose

Defines how maintainers and coding agents audit, write, generate, review, and cross-link g8e documentation. The current working tree is the source of truth for current behavior. Historical release notes and evidence artifacts describe only their stated release, run, or assessment scope.

## Quick index

- [Purpose](#purpose)
- [Quick index](#quick-index)
- [Invariants](#invariants)
- [Owned surfaces](#owned-surfaces)
- [Procedures](#procedures)
- [Anti-patterns](#anti-patterns)
- [Links out](#links-out)

Invariant groups: [Format and metadata](#format-and-metadata-inv-doc-fmt), [Factual authority](#factual-authority-inv-doc-auth), [Generated outputs](#generated-outputs-inv-doc-gen), [Style and structure](#style-and-structure-inv-doc-style).

## Invariants

Ids are stable. Append the next free number in a topic. Do not renumber.

### Format and metadata (`INV-DOC-FMT`)

| ID | Rule |
| --- | --- |
| INV-DOC-FMT-01 | Maintained developer files in `docs/devs/` MUST use the dev docs format with YAML front matter (`doc_id`, `title`, `audience`, `status`, `last_updated`, `version`, `owners`, `related`, `when_to_read`, `do_not_use_for`) and standard H2 sections: Purpose, Quick index, Invariants, Owned surfaces, Procedures, Anti-patterns, Links out. |
| INV-DOC-FMT-02 | `last_updated` and `version` metadata MUST be updated only after completing the full [End-to-End Audit Workflow](#end-to-end-audit-workflow). `version` MUST match the exact string in `VERSION`. |
| INV-DOC-FMT-03 | Documents evaluated for impact during a change or release that require no edits MUST retain their existing metadata. Blanket-bumping metadata without an audit is prohibited. |
| INV-DOC-FMT-04 | Historical release notes in `docs/release_notes/` and evidence artifacts are immutable and MUST retain the version, run ID, and timestamps of their original scope. |
| INV-DOC-FMT-05 | Maintained protocol specifications under `protocol/docs/` (`a2a.md`, `constants.md`, `mcp.md`, `spec.md`) MUST include a `Version: vX.Y.Z` header matching the exact string in `VERSION`. |

### Factual authority (`INV-DOC-AUTH`)

| ID | Rule |
| --- | --- |
| INV-DOC-AUTH-01 | Every behavioral claim MUST resolve to current code, configuration, schemas, tests, or scope-bound evidence. Existing prose is never proof of current behavior. |
| INV-DOC-AUTH-02 | CLI command names, flags, defaults, and usage MUST match `./g8e <command> --help`. Docs MUST NOT maintain duplicated flag dumps that drift from the code. |
| INV-DOC-AUTH-03 | Each concept MUST have one canonical explanation. Related documents route readers to the canonical owner via cross-links rather than duplicating content. |
| INV-DOC-AUTH-04 | Security and governance claims MUST state posture, ingress path, trust boundary, identity requirements, and explicit limitations. Broad claims of absolute security or external certification are prohibited. |

### Generated outputs (`INV-DOC-GEN`)

| ID | Rule |
| --- | --- |
| INV-DOC-GEN-01 | Generated protobuf references in `protocol/docs/reference/api/` MUST change only through edits to `.proto` files in `protocol/proto/g8e/` followed by `make proto-generate` (or `make proto-go`). MUST NOT hand-edit generated protobuf output. |
| INV-DOC-GEN-02 | Gateway OpenAPI specifications (`internal/services/gateway/docs/swagger.json` and `swagger.yaml`) MUST change only through Go Swagger annotations followed by `make swagger-generate`. `swagger.json` is embedded into the Gateway binary via `internal/services/gateway/docs/docs.go` and verified by `go test ./internal/services/gateway/docs`. |
| INV-DOC-GEN-03 | Machine-readable protocol constants and schemas under `protocol/constants/`, `protocol/models/`, and `protocol/schemas/` MUST remain synchronized with their Go, Python, and TypeScript mirrors via owning validation commands (`make constants-check`, `make doctrines-validate`, `make agent-tool-registry-check`, `make explorer-catalog-check`). |
| INV-DOC-GEN-04 | Static website documentation in `website/` MUST be rendered from root `README.md` via `make website-build` and validated via `make website-test`. MUST NOT hand-edit rendered website output. |
| INV-DOC-GEN-05 | Release compliance evidence projections (`docs/release_notes/vX.Y.x/*-compliance-evidence.md` and `.csv`) MUST be generated through `./g8e compliance release-prepare` or `./g8e compliance release-evidence`. MUST NOT hand-edit projected compliance evidence. |

### Style and structure (`INV-DOC-STYLE`)

| ID | Rule |
| --- | --- |
| INV-DOC-STYLE-01 | Prose MUST use present tense, active voice, and exact technical terminology. Source lines MUST NOT be hard-wrapped. |
| INV-DOC-STYLE-02 | Code references MUST use repository-relative paths and stable symbol names. Line-number references and machine-absolute paths are prohibited. |
| INV-DOC-STYLE-03 | Relative links MUST resolve from the directory containing the document. Link syntax MUST use `[text](target.md)`. |
| INV-DOC-STYLE-04 | Standalone component and guide documents MUST link to their canonical architecture or index document. Component index documents MUST list all maintained documents in their set. |

## Owned surfaces

### Documentation surfaces

| Claim | Path | Verify |
| --- | --- | --- |
| Dev docs format | `docs/devs/` | YAML front matter and required H2 section order |
| Documentation catalog | `docs/devs/docs.md` | Authoritative inventory of all first-party docs |
| Protobuf API references | `protocol/proto/g8e/`, `protocol/docs/reference/api/` | `make proto-generate` (or `make proto-go`) |
| Gateway OpenAPI | `internal/services/gateway/docs/`, Go Swagger annotations | `make swagger-generate`, `go test ./internal/services/gateway/docs` |
| Protocol events and constants | `protocol/constants/events.json`, `internal/constants/` | `make constants-generate`, `make constants-check` |
| Protocol doctrines | `protocol/constants/doctrine/*.json` | `make doctrines-validate` |
| Agent tool registry | `protocol/constants/agenttools/agent-tool-registry.json` | `make agent-tool-registry-generate`, `make agent-tool-registry-check` |
| Explorer scenario catalog | `evaluation-explorer/` (`scenario-catalog.generated.ts`) | `make explorer-catalog-generate`, `make explorer-catalog-check` |
| Static website | `README.md`, `website/` | `make website-build`, `make website-test` |
| Compliance evidence projections | `docs/release_notes/vX.Y.x/` | `./g8e compliance release-prepare`, `./g8e compliance report verify` |

### Documentation catalog

This catalog provides the authoritative inventory of all first-party documentation across the repository.

#### Repository entry points and governance

- [Root README](../../README.md): Product overview, evidence boundaries, quick start, architecture summary, and documentation entry routes.
- [Changelog](../../CHANGELOG.md): Release index documenting changes across versions with links to per-release notes.
- [Early Testers Guide](../../EARLY_TESTERS.md): Program guide and onboarding instructions for early testers.
- [Contributing](../../.github/CONTRIBUTING.md): Contribution workflow, PR expectations, and developer entry points.
- [Code of Conduct](../../.github/CODE_OF_CONDUCT.md): Contributor code of conduct.
- [Security Policy](../../.github/SECURITY.md): Vulnerability disclosure procedures and supported release versions.
- [Pull Request Template](../../.github/pull_request_template.md): Required change summary, test evidence, and verification checklist.

#### Core platform concepts and position papers

- [About g8e](../core/about.md): High-level mission, platform philosophy, and design principles.
- [Position Paper](../core/position_paper.md): Conceptual governance framework, architectural rationale, and trust models.

#### Developer documentation (`docs/devs/`)

- [Developer Guidelines](devs.md): Go platform coding invariants, CLI layout, error contracts, and runtime guidelines.
- [Code Map](codemap.md): Package and runtime ownership maps, directory structure, and substrate separation.
- [Documentation Guide](docs.md): Documentation invariants, audit workflows, generated outputs, and documentation catalog.
- [Testing Guide](tests.md): Four-tier test model, execution commands, fixture lifecycles, and testing invariants.
- [Release Process](release_process.md): Version synchronization, release compliance evidence, smoke gates, and release publication workflow.
- [Troubleshooting](troubleshooting.md): Diagnostics, known failure modes, and recovery procedures.
- [Python Linting](python-linting.md): Python linting audit record and type-safety configuration reference.

#### Platform architecture (`docs/architecture/`)

- [System Architecture Overview](../architecture/overview.md): System topology, component boundaries, and overall platform architecture.
- [Gateway Architecture](../architecture/gateway.md): Gateway service architecture, HTTP routing, PKI, and control-plane dispatch.
- [Operator Architecture](../architecture/operator.md): Outbound and embedded Operator architecture, task execution, and host lifecycle.
- [Governance Architecture](../architecture/governance.md): Five-layer governance framework (L1 doctrine to L5 audit), posture policies, and enforcement.
- [AI Agents and Governance Boundary](../architecture/agents.md): AI agent integration patterns, tool boundaries, and trust limits.
- [Consensus Architecture](../architecture/consensus.md): Quorum, multi-party consensus, and state agreement mechanisms.
- [Storage Architecture](../architecture/storage.md): SQLite persistence, schema migrations, and storage service layout.
- [Events Architecture](../architecture/events.md): Event taxonomy, delivery guarantees, and event bus lifecycle.
- [Evaluation Architecture](../architecture/evals.md): Native evaluation framework, campaign specifications, scoring, and evidence capture.
- [Encryption Architecture](../architecture/encryption.md): Cryptographic primitives, key derivation, envelope encryption, and key management.
- [Authentication Architecture](../architecture/auth.md): Authentication, passkeys/WebAuthn, session management, and workload identity.
- [Network Architecture](../architecture/network.md): Network topologies, port management, listener isolation, and mTLS.
- [Protocol Architecture](../architecture/protocol.md): Protocol wire formats, serialization, envelope hashing, and schema compatibility.
- [Public Spectator Architecture](../architecture/public_spectator.md): Read-only public feed spectator service and event broadcasting.
- [Server-Sent Events Architecture](../architecture/sse.md): Server-Sent Events architecture for streaming LLM tokens and live platform updates.
- [Console Architecture & Development](../architecture/console.md): Embedded browser console architecture, state management, and API integration.
- [Ensemble Architecture](../architecture/ensemble.md): Ensemble multi-model runtime architecture, provider isolation, and worker dispatch.
- [Model Provenance](../architecture/model-provenance.md): Model attribution, weights tracking, and supply-chain provenance.
- [Scripts Architecture](../architecture/scripts.md): Administrative and automation scripts architecture and execution environment.

#### Operator and deployment guides (`docs/guides/`)

- [Getting Started](../guides/getting_started.md): End-to-end local platform installation, onboarding, and first execution.
- [Air-Gapped Deployment](../guides/air_gap.md): Air-gapped and disconnected deployment guide and local caching procedures.
- [Unified Docker Stack](../guides/unified_stack.md): Docker Compose unified stack deployment and lifecycle management.
- [Docker Gateway](../guides/docker_gateway.md): Containerized Gateway operations, volume management, and environment configuration.
- [Build Gateway](../guides/build_gateway.md): Compiling and packaging the Gateway binary from source.
- [Build Operator](../guides/build_operator.md): Building and packaging the standalone Operator container.
- [Connect Operator to Gateway](../guides/connect_operator_to_gateway.md): Operator enrollment and mTLS connection to the Gateway.
- [Connect Applications to Gateway](../guides/connect_apps_to_gateway.md): Connecting client applications and SDKs to the Gateway.
- [Connect Frontend to Gateway](../guides/connect_frontend_to_gateway.md): Connecting custom frontend applications to Gateway APIs.
- [Build Frontend](../guides/build_frontend.md): Building and deploying frontend user interfaces.
- [Build Applications](../guides/build_apps.md): Building custom applications atop the g8e platform.
- [Build Observe Frontend](../guides/build_observe_frontend.md): Building custom telemetry and observation frontends with g8e-adapter.
- [Cloudflare Tunnel](../guides/cloudflare_tunnel.md): Configuring Cloudflare Tunnels for secure ingress.
- [Public Spectator Guide](../guides/public_spectator.md): Setting up and operating public spectator feeds.
- [Reset Workload Identity](../guides/reset_workload_identity.md): Resetting and re-enrolling workload identity credentials.
- [Sovereignty Gauntlet](../guides/sovereignty_gauntlet.md): Step-by-step verification procedures for sovereignty and governance controls.
- [Lovable Integration](../guides/lovable.md): Integrating with Lovable.dev low-code applications.
- [Host-Native Headless UX Smoke Test](../guides/ux_smoke_test.md): Retired host-native UX smoke test runbook (retained as historical/retired status).

#### Reference and compliance (`docs/reference/`)

- [Glossary](../reference/glossary.md): Canonical platform definitions, acronyms, and terminology.
- [Compliance Alignment](../reference/compliance-alignment.md): Mapping of g8e controls to NIST, SOC 2, ISO 27001, and COSAiS frameworks.
- [Compliance Evidence](../reference/compliance-evidence.md): Evidence collection requirements, schema definitions, and audit verification.
- [FIPS 140-3 Compliance](../reference/fips140-3.md): FIPS 140-3 cryptographic module boundary, build flags, and verification instructions.

#### Architecture diagrams (`docs/diagrams/`)

- [System Overview Flowchart](../diagrams/flowchart-system-overview-lr.md): Mermaid flowchart showing top-level system architecture.
- [Gateway Fleet Graph](../diagrams/graph-gateway-fleet-single-host-http-mtls.md): Fleet network topology, port allocations, and mTLS trust boundaries.
- [Gateway Services Graph](../diagrams/graph-gateway-services.md): Internal Gateway services, dependency graph, and middleware dispatch.
- [Operator Lifecycle Graph](../diagrams/graph-operator-lifecycle.md): Operator lifecycle state machine from bootstrap to shutdown.
- [Operator Pipeline L1-L5 Graph](../diagrams/graph-operator-pipeline-l1-l5.md): L1-L5 execution pipeline through the Operator engine.
- [50k System Graph](../diagrams/graph-system-50k.md): 50,000-foot component graph and protocol interaction boundaries.
- [Sequence Principal Ensemble Gateway Operator v3](../diagrams/sequence-principal-ensemble-gateway-operator-v3.md): End-to-end sequence diagram for governed execution.

#### Ensemble subsystem documentation (`docs/ensemble/` and `ensemble/`)

- [Ensemble Package README](../../ensemble/README.md): Ensemble package overview, installation, and local development setup.
- [Ensemble Changelog](../../ensemble/CHANGELOG.md): Ensemble version history and release notes.
- [Ensemble Contribution Guide](../../ensemble/CONTRIBUTING.md): Python and Ensemble contributor guide.
- [Ensemble Documentation Index](../ensemble/index.md): Subsystem navigation index for all Ensemble documentation.
- [Ensemble Architecture](../ensemble/architecture.md): Ensemble service architecture, FastAPI routing, and worker pool design.
- [Ensemble Agents](../ensemble/agents.md): Agent abstractions, tools, and execution contexts.
- [Ensemble Getting Started](../ensemble/getting-started.md): Developer quick start for running and extending Ensemble.
- [Ensemble Developer Guide](../ensemble/devs.md): Python development conventions, virtual environments, and typing standards.
- [Ensemble Governance](../ensemble/governance.md): Integration between Ensemble agent steps and Gateway governance.
- [Ensemble Decision Providers](../ensemble/decision-providers.md): Provider abstraction for voting and consensus decisions.
- [Ensemble LLM Providers](../ensemble/llm-providers.md): Configuration and drivers for Ollama, OpenAI, Anthropic, Gemini, and local models.
- [Ensemble PKI](../ensemble/pki.md): Ensemble mTLS certificate handling and workload identity.
- [Ensemble Prompts](../ensemble/prompts.md): Prompt engineering patterns, templates, and frozen vectors.
- [Ensemble Protocol](../ensemble/protocol.md): Ensemble protocol implementations and RPC handlers.
- [Ensemble SSE](../ensemble/sse.md): Token and event streaming over Server-Sent Events in Ensemble.
- [Ensemble Storage](../ensemble/storage.md): Local persistence, SQLite caching, and conversation memory.
- [Ensemble Tests](../ensemble/tests.md): Pytest test organization, marks, fixtures, and execution.
- [Ensemble Thinking](../ensemble/thinking.md): Deep thinking and reasoning output streaming and inspection.

#### Protocol specifications and references (`protocol/`)

- [Protocol README](../../protocol/README.md): Protocol package overview, directory layout, code generation, and version synchronization.
- [Protocol Specification](../../protocol/docs/spec.md): Authoritative wire protocol specification.
- [MCP Specification](../../protocol/docs/mcp.md): Model Context Protocol integration and wire contract.
- [A2A Specification](../../protocol/docs/a2a.md): Agent-to-Agent communication protocol specification.
- [Constants Specification](../../protocol/docs/constants.md): Protocol constant registries and semantics.
- [Generated Common API](../../protocol/docs/reference/api/g8e/common/v1/index.md): Common protocol messages, error envelopes, and base types.
- [Generated Compliance API](../../protocol/docs/reference/api/g8e/compliance/v1/index.md): Compliance report, evidence, and audit messages.
- [Generated Eval API](../../protocol/docs/reference/api/g8e/eval/v1/index.md): Evaluation campaign, scenario, and metrics messages.
- [Generated Operator API](../../protocol/docs/reference/api/g8e/operator/v1/index.md): Operator task, execution, and receipt messages.
- [Generated PubSub API](../../protocol/docs/reference/api/g8e/pubsub/v1/index.md): Pub/sub message contracts and event envelopes.
- [Protocol Python Package](../../protocol/python/README.md): Python protocol bindings, stubs, and package installation.
- [Protocol Conformance](../../protocol/conformance/README.md): Conformance test harness and cross-language compatibility tests.

#### Subsystem runbooks and adapters

- [Evaluation Explorer Runbook](../../evaluation-explorer/RUNBOOK.md): Operational runbook for developing and building the Evaluation Explorer SPA.
- [Evaluation Campaign README](../../eval/README.md): Evaluation campaign data layout, fixtures, and directory conventions.
- [Frontend Adapter Contract Pack](../../g8e-adapter/contract-pack/README.md): Audited frontend adapter contract pack reference and integration guide.
- [Frontend Contract Pack Builder Prompt](../../g8e-adapter/contract-pack/builder-prompt.md): AI prompt specification for generating conforming observe frontends.
- [Examples README](../../examples/README.md): Platform example applications index and runnable samples.
- [External Console Example](../../examples/external-console/README.md): External standalone console sample application.

#### Release notes and evidence projections (`docs/release_notes/`)

- [Release Notes Directory](../release_notes/): Immutable per-release notes organized by minor version directory (`v0.1.x/` through `v2.3.x/`), alongside scoped release compliance evidence projections (`*-compliance-evidence.md` and `.csv`).

## Procedures

### End-to-End Audit Workflow

1. Read the complete document from beginning to end.
2. Classify document purpose (guide, architecture, reference, developer, protocol, historical).
3. Inventory every claim (commands, flags, routes, defaults, ports, paths, contracts).
4. Trace each claim to its primary owning source in the repository using the [Source-of-Truth Traceability Matrix](#source-of-truth-traceability-matrix).
5. Inspect related documentation to update cross-links and eliminate conflicting duplicates.
6. Refresh generated outputs through source owners (`make proto-generate`, `make swagger-generate`, `make constants-generate`, `make agent-tool-registry-generate`, `make explorer-catalog-generate`, `make website-build`).
7. Review revised document end-to-end for coherence, relative links, and clean formatting without hard-wrapped lines.
8. Update `last_updated` and `version` metadata last.
9. Run owning validation commands (`./g8e test lint`, component tests, or schema validators).

### Source-of-Truth Traceability Matrix

| Claim | Primary owners | Required corroboration |
| --- | --- | --- |
| CLI commands, arguments, flags, defaults, and usage | Cobra definitions in `internal/cli/cmd/` | Relevant `./g8e <command> --help`, command tests, and CLI wiring |
| HTTP routes, methods, and route authentication | Canonical paths in `protocol/constants/api_paths.json` and router/controller registration in `internal/services/gateway/` | Route authentication inventory, middleware, controller behavior, and `internal/services/gateway/docs/swagger.json` |
| Authentication and identity | Gateway auth middleware, `internal/services/auth/`, CLI enrollment code, PKI services, and protocol identity helpers | Route classification, certificate/session validation, enrollment tests, and deployed topology |
| Configuration and environment | Types, defaults, and validation in `internal/config/`, CLI flags, environment constants in `protocol/constants/env_vars.json`, and component settings | Startup construction, Compose configuration, Docker entrypoints, and configuration tests |
| Governance and execution | `internal/services/governance/`, `internal/services/consensus/`, `internal/services/pubsub/`, `internal/services/mcp/`, and construction in Gateway and Operator startup | Active posture branches, complete ingress-to-dispatch path, persistence, rejection behavior, and integration tests |
| Persistence and runtime files | Owning store or service and `internal/services/fs/` for `.g8e/` state | Path constants, schema lifecycle, permissions, cleanup behavior, and tests using isolated runtime roots |
| Wire messages and gRPC services | Protobuf schemas in `protocol/proto/g8e/`, model schemas in `protocol/models/`, and canonicalization implementations | Generated bindings, vectors, contract tests, and cross-language conformance tests |
| Protocol identifiers and registries | Registry-specific source declaration in `protocol/constants/` or `internal/constants/` | Mirrors, consumers, package loaders, and contract tests; verify owning generator and check target |
| Gateway OpenAPI | Router/controller behavior and Go Swagger annotations in `cmd/g8e`, `internal/services/gateway/`, `internal/models/`, `internal/constants/` | Generated `swagger.json` and `swagger.yaml`, route registration, authentication behavior, and `go test ./internal/services/gateway/docs` |
| Ensemble behavior | `ensemble/app/`, typed models and settings, `pyproject.toml`, and startup wiring | Pytest suites, provider boundaries, Gateway/Operator integration, and `docs/ensemble/` index |
| Deployment and ports | Root and component Dockerfiles, `docker-compose.yml`, entrypoints, and CLI Docker orchestration | Profiles, health checks, volumes, identity enrollment, network exposure, and documented commands |
| Tests and CI | `internal/cli/cmd/test/`, root and component Makefiles, package test configuration, and `.github/workflows/` | Actual target dependencies, build tags, exclusions, timeouts, race settings, and external-service gates |
| Compliance, cryptographic, and measured claims | Exact signed artifact, verifier, catalog, schema, or evidence graph named by the claim | Version, scope, environment, trust inputs, evidence cutoff, failure cases, and explicit limitations |

### Refresh Generated Documentation

```bash
# Protobuf references, Go/Python/Node code, downstream lockfiles
make proto-generate

# Gateway Swagger / OpenAPI JSON and YAML
make swagger-generate

# Validate and generate protocol events and Go constants
make constants-generate
make constants-check

# Validate doctrine JSON schemas
make doctrines-validate

# Sync agent tool registry and explorer scenario catalog
make agent-tool-registry-generate
make agent-tool-registry-check
make explorer-catalog-generate
make explorer-catalog-check

# Build and verify rendered website documentation
make website-build
make website-test
```

## Anti-patterns

- Hand-editing generated protobuf references in `protocol/docs/reference/api/` or Swagger JSON without updating sources (INV-DOC-GEN-01, INV-DOC-GEN-02).
- Updating `last_updated` or `version` without auditing the full document (INV-DOC-FMT-02).
- Hard-wrapping source prose lines or embedding source code line numbers (INV-DOC-STYLE-01, INV-DOC-STYLE-02).
- Copying full CLI flag inventories or large schema tables instead of linking (INV-DOC-AUTH-02, INV-DOC-AUTH-03).
- Claiming third-party certifications or unqualified security guarantees (INV-DOC-AUTH-04).
- Hand-editing generated agent tool registries or explorer scenario catalogs without updating their owning source (INV-DOC-GEN-03).
- Hand-editing projected compliance evidence in release notes instead of generating via `./g8e compliance release-prepare` (INV-DOC-GEN-05).

## Links out

- [Developer Guidelines](devs.md): coding invariants and repository standards.
- [Code Map](codemap.md): package and runtime ownership maps.
- [Testing Guide](tests.md): test execution tiers and testing invariants.
- [Release Process](release_process.md): versioning, compliance bundles, and release workflow.
- [Troubleshooting](troubleshooting.md): diagnostics and recovery procedures.
- [Python Linting](python-linting.md): Python type safety and linting audit reference.
