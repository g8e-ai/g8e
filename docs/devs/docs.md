# Documentation Guide

Last Updated: 2026-09-08
Version: v2.1.7

This guide defines how maintainers and AI agents audit, write, generate, review, and cross-link g8e documentation. The current working tree is the source of truth for current behavior. Historical release notes and evidence artifacts describe only their stated release, run, or assessment scope.

## Documentation Standard

Every documentation change is a code change. A complete update verifies the entire affected document against the implementation, updates every related current-state document, preserves the boundaries of historical and evidence-bearing material, refreshes generated outputs through their owners, and increments document metadata only after the audit is complete.

Documentation in this repository is:

- **Factual:** Every behavioral claim resolves to current code, configuration, schemas, generated output, tests, or explicitly scoped evidence.
- **Complete for its purpose:** A task guide includes prerequisites, commands, expected results, security requirements, and recovery paths. A reference defines its complete public contract or links to the canonical generated contract. An architecture document names boundaries, ownership, data flow, and limitations.
- **High signal:** Each fact has one canonical explanation. Other documents provide enough context to route the reader there.
- **Current:** Maintained documents describe present behavior. Release notes and evidence remain explicitly historical or scope-bound.
- **Discoverable:** Every first-party document belongs to a cataloged documentation surface and links to its parent index or a closely related canonical document.

## End-to-End Audit Workflow

Use this sequence for every reviewed document. Do not change `Last Updated` or `Version` until steps 1 through 7 are complete.

1. **Read the complete document.** Review the title, metadata, prose, tables, examples, diagrams, footnotes, links, and generated sections from beginning to end. Do not patch only the reported paragraph.
2. **Classify the document.** Identify it as task-oriented, architecture, reference, developer, protocol, generated, historical, evidence-bearing, or repository-governance documentation. A document can have more than one classification, but one purpose remains primary.
3. **Inventory every claim.** Check commands, flags, routes, authentication requirements, fields, status values, defaults, ports, paths, identities, component names, security properties, test behavior, deployment topology, and evidence statements.
4. **Trace each claim to its owner.** Read the production path from construction through enforcement, persistence, and error propagation. Use tests as executable corroboration, not as a substitute for the current implementation.
5. **Inspect related documentation.** Search for the same concept, symbol, command, route, or claim. Select one canonical explanation, update stale summaries, and add links instead of preserving conflicting copies.
6. **Update source-owned and generated material correctly.** Edit source annotations, schemas, templates, or reviewed evidence inputs before running the relevant generator. Never hand-edit a generated output to conceal source drift.
7. **Review the revised document end to end.** Confirm that the structure is coherent, examples are internally consistent, limitations are explicit, links resolve, prose is not hard-wrapped, and no stale text remains.
8. **Update metadata last.** For a maintained document that carries `Last Updated` and `Version`, set the date to the completed audit date and the version to the exact value in `VERSION`. Metadata certifies that the document was reviewed for that repository version; it is not a substitute for the audit.
9. **Run the owning validation.** Run the narrow generator, tests, build, or validation commands for the changed surfaces, then reread the final generated and handwritten output.

A partial review does not qualify for a metadata update. If the available source or environment cannot substantiate a claim, narrow or remove the claim and record the limitation in the document.

## Write for the Reader

### Task-oriented guides

Task-oriented guides help users install, authenticate, configure, integrate with, and operate g8e.

- Lead with prerequisites, exact commands, expected results, and recovery steps.
- Include runnable command, configuration, request, and response examples when they materially help complete the task.
- Define public flags, routes, fields, status values, identity requirements, trust inputs, and destructive effects precisely.
- Explain only the architecture required to perform the procedure safely.
- Link to canonical references for schemas, endpoint inventories, posture semantics, and internal design.

### Architecture and reference documentation

Architecture and reference documents explain system boundaries, responsibilities, public interfaces, protocol behavior, data flow, persistence, and security properties.

- Name the component, trust boundary, public interface, wire type, storage owner, and enforced checks.
- Distinguish Gateway in-process execution, outbound Operator execution, direct-envelope submission, MCP and A2A ingress, Operator command relay, and the external MCP wrapper. These paths do not provide identical governance behavior.
- State bypass and scope limits. Client-native tools and side channels do not become governed merely because the client also connects to g8e.
- Link to generated protobuf and OpenAPI references rather than copying large schemas or endpoint tables.
- Keep one canonical explanation for posture and five-layer behavior in [Governance](../architecture/governance.md), with integration-path limitations in [AI Agents and the g8e Governance Boundary](../architecture/agents.md).

### Developer and protocol documentation

Developer documentation in `docs/devs/`, component development guides, and protocol specifications may include implementation details needed to change the system safely.

- Use repository-relative source paths and stable package, type, function, command, and message names.
- Document construction order, persistence, error propagation, test infrastructure, generated artifacts, and internal contracts when they affect maintenance.
- Keep wire requirements in `protocol/docs/`; keep implementation and contribution guidance in `docs/devs/` or the owning component documentation.
- Link code ownership and package locations to the [Code Map](codemap.md) rather than copying a second repository map.

### Historical and evidence-bearing documentation

Release notes preserve the behavior and scope of their release. Evidence documentation preserves the environment, trust inputs, artifacts, results, and limitations of the cited assessment or run.

- Do not rewrite historical documents as current-state guides.
- Do not silently correct an old claim by changing its scope. Add a clearly dated correction or update the current canonical documentation.
- Never promote architecture, catalog coverage, CI status, or a bounded test result into certification, operating effectiveness, broad model quality, zero-leakage, or production-suitability claims.
- Keep public README evidence checksum-bound and release-owner-approved through the projection and promotion process in the [Release Process](release_process.md).

## Source-of-Truth Matrix

Trace each claim through all applicable owners. A constant, schema, test, annotation, or generated file alone does not prove runtime behavior.

| Claim | Primary owners | Required corroboration |
| --- | --- | --- |
| CLI commands, arguments, flags, defaults, and output | Cobra definitions in `internal/cli/cmd/` | Relevant `./g8e ... --help`, command tests, and startup wiring |
| HTTP routes and methods | Canonical path constants and router/controller registration in `internal/services/gateway/` | Route authentication inventory, middleware, controller behavior, and integration tests |
| Authentication and identity | Gateway auth middleware and controllers, CLI enrollment code, PKI services, and protocol identity helpers | Route classification, certificate/session validation, enrollment tests, and deployed topology |
| Configuration and environment | Types, defaults, and validation in `internal/config/`, CLI flags, environment constants, and component settings | Startup construction, Compose configuration, Docker entrypoints, and configuration tests |
| Governance and execution | `internal/services/governance/`, `internal/services/consensus/`, `internal/services/pubsub/`, `internal/services/mcp/`, and construction in Gateway and Operator startup | Active posture branches, complete ingress-to-dispatch path, persistence, rejection behavior, and integration tests |
| Persistence and runtime files | Owning store or service and `internal/services/fs/` for `.g8e/` state | Path constants, schema lifecycle, permissions, cleanup behavior, and tests using isolated runtime roots |
| Wire messages and canonical serialization | Protobuf schemas in `protocol/proto/g8e/`, model schemas in `protocol/models/`, and canonicalization implementations | Generated bindings, vectors, contract tests, and cross-language conformance tests |
| Protocol identifiers and registries | Registry-specific source declaration in `protocol/constants/` or `internal/constants/` | Mirrors, consumers, package loaders, and contract tests; do not assume one ownership direction for every registry |
| Gateway OpenAPI | Router/controller behavior and Go Swagger annotations | Generated `swagger.json` and `swagger.yaml`, route registration, authentication behavior, and relevant tests |
| Dashboard behavior | `dashboard/server.js`, `dashboard/public/`, enrollment services, package metadata, and Compose wiring | Vitest coverage and a browser/runtime trace; retained UI modules do not prove deployed availability |
| Ensemble behavior | `ensemble/app/`, typed models and settings, `pyproject.toml`, and startup wiring | Pytest suites, provider boundaries, Gateway/Operator integration, and the component documentation index |
| Deployment and ports | Root and component Dockerfiles, `docker-compose.yml`, demo Compose files, entrypoints, and CLI Docker orchestration | Profiles, health checks, volumes, identity enrollment, network exposure, and the exact documented command |
| Tests and CI | `internal/cli/cmd/test.go`, root and component Makefiles, package test configuration, and `.github/workflows/` | Actual target dependencies, build tags, exclusions, timeouts, race settings, and external-service gates |
| Compliance, cryptographic, and measured claims | Exact signed artifact, verifier, catalog, schema, or evidence graph named by the claim | Version, scope, environment, trust inputs, evidence cutoff, failure cases, and explicit limitations |

When sources disagree, follow the current runtime path, identify the owning source, and update stale current-state mirrors that are in scope. Never choose a source merely because it supports the existing prose.

## Generated and Machine-Readable Documentation

| Output | Source and ownership | Update and validation |
| --- | --- | --- |
| Root `README.md` | `docs/templates/README.md.tmpl` plus the reviewed snapshot in `docs/evidence/readme/current/`, rendered by `scripts/generate_readme.py` | Edit the template for stable prose. Update evidence only through the attended projection and digest-approved promotion flow. Run `make readme`, `make readme-test`, and `make readme-check`. |
| Public README evidence | Private immutable eval report projected by `scripts/project_readme_evidence.py`; Stage 2 provenance collected by `scripts/collect_readme_provenance.py`; reviewed candidate promoted by `scripts/promote_readme_evidence.py` | Follow [Release Process](release_process.md). CI validates checked-in evidence and README drift but does not collect, select, approve, or promote evidence. |
| Protobuf API reference | Comments and definitions in `protocol/proto/g8e/`, generated by the `protoc-gen-doc` entry in root `buf.gen.yaml` | Edit `.proto` files and run `make proto`. This also refreshes Go, Python, TypeScript, Markdown, and downstream lockfile outputs owned by the target. |
| Gateway OpenAPI | Go Swagger annotations in `cmd/g8e`, `internal/services/gateway`, `internal/models`, and `internal/constants` | Run `make swagger-generate`. `internal/services/gateway/docs/docs.go` embeds `swagger.json`; `swagger.yaml` is a generated companion. |
| Website | Root `README.md` consumed by the generator in `website/` | Run `make website-test` and `make website-build` after README changes that affect site rendering. |
| Protocol constants, models, and compliance catalogs | JSON registries and schemas under `protocol/` with registry-specific Go/Python consumers and contract tests | Run the protocol, conformance, doctrine, COSAiS, or compliance validation owned by the changed surface. JSON reference data is machine-readable documentation and receives the same factual review as prose. |

Never hand-edit generated outputs without changing their owner. Never write directly to `docs/evidence/readme/current/`; promotion installs only a validated candidate whose canonical tree digest received explicit release-owner approval.

## Documentation Catalog

This catalog covers every first-party documentation surface in the repository. Imported material under `vendor/` and third-party source documentation under `third_party/` retain upstream ownership and are not current g8e product documentation.

### Repository entry points and governance

- [Root README](../../README.md): Generated product overview, evidence boundaries, quick start, architecture summary, and documentation routes. Stable prose lives in [the README template](../templates/README.md.tmpl).
- [Changelog](../../CHANGELOG.md): Release index and links to per-release notes.
- [Contributing](../../.github/CONTRIBUTING.md): Contribution workflow and documentation entry point.
- [Security policy](../../.github/SECURITY.md): Supported versions and vulnerability reporting.
- [Code of Conduct](../../.github/CODE_OF_CONDUCT.md): Contributor conduct policy.
- [Pull request template](../../.github/pull_request_template.md): Required change and verification summary.
- [Root license](../../LICENSE), [protocol license](../../protocol/LICENSE), [Ensemble license](../../ensemble/LICENSE), and [Dashboard license](../../dashboard/LICENSE): Legal terms for the repository and separately packaged components.

### Platform concepts and architecture

- [About](../core/about.md) and [Position Paper](../core/position_paper.md): Product scope, system model, and design rationale.
- [Architecture Overview](../architecture/overview.md): Platform components, trust boundaries, and end-to-end flow.
- [Gateway](../architecture/gateway.md), [Operator](../architecture/operator.md), [Ensemble](../architecture/ensemble.md), and [Dashboard](../architecture/dashboard.md): Component-level runtime architecture.
- [Governance](../architecture/governance.md), [Consensus](../architecture/consensus.md), and [AI Agents and the Governance Boundary](../architecture/agents.md): Postures, five-layer enforcement, L2, ingress paths, and client-side limits.
- [Authentication and Authorization](../architecture/auth.md), [Encryption](../architecture/encryption.md), and [Network](../architecture/network.md): Identity, PKI, cryptography, transport, and topology.
- [Protocol](../architecture/protocol.md), [Storage](../architecture/storage.md), and [SSE](../architecture/sse.md): Wire contracts, persistence ownership, and event transport.
- [Scripts](../architecture/scripts.md): Setup-script behavior and platform support.

### User and operator guides

- [Getting Started](../guides/getting_started.md), [Unified Stack](../guides/unified_stack.md), and [Docker Gateway](../guides/docker_gateway.md): Primary installation, deployment, bootstrap, and owner-approval workflows.
- [Build Gateway](../guides/build_gateway.md), [Build Operator](../guides/build_operator.md), [Connect Operator](../guides/connect_operator_to_gateway.md), and [Air Gap](../guides/air_gap.md): Core component deployment and disconnected operation.
- [Build Apps](../guides/build_apps.md), [Connect Apps](../guides/connect_apps_to_gateway.md), [Build Frontend](../guides/build_frontend.md), and [Connect Frontend](../guides/connect_frontend_to_gateway.md): Public client and browser integration paths.
- [Cloudflare Tunnel](../guides/cloudflare_tunnel.md) and [Lovable](../guides/lovable.md): Optional external frontend and tunnel integration.
- [Sovereignty Gauntlet](../guides/sovereignty_gauntlet.md): Evidence-oriented demonstration and README evidence collection workflow.
- [UX Smoke Test](../guides/ux_smoke_test.md): Manual product-surface verification.

### Developer documentation

- [Developer Guidelines](devs.md): Coding, error, path, runtime file, testing, and contribution rules.
- [Code Map](codemap.md): Current repository ownership by runtime entry point and package.
- [Testing](tests.md): Four-tier test model, fixtures, commands, and CI scope.
- [Release Process](release_process.md): Versioning, release evidence, README evidence projection and promotion, and publication procedure.
- [Troubleshooting](troubleshooting.md): Maintainer diagnosis and recovery.
- This [Documentation Guide](docs.md): Documentation ownership, audit process, catalog, style, generation, and validation.

### Dashboard documentation

The [g8ed index](../dashboard/index.md) owns the component documentation map and current capability status. Its complete set is [Architecture](../dashboard/architecture.md), [Authentication](../dashboard/auth.md), [Gateway Integration](../dashboard/gateway.md), [Operator Surfaces](../dashboard/operators.md), [SSE](../dashboard/sse.md), [Development](../dashboard/development.md), and [Testing](../dashboard/tests.md). The component entry point is [dashboard/README.md](../../dashboard/README.md).

### Ensemble documentation

The [g8ee index](../ensemble/index.md) owns the component documentation map. Its complete set is [Getting Started](../ensemble/getting-started.md), [Architecture](../ensemble/architecture.md), [Governance](../ensemble/governance.md), [Agents](../ensemble/agents.md), [Protocol](../ensemble/protocol.md), [Prompts](../ensemble/prompts.md), [Thinking](../ensemble/thinking.md), [PKI and Trust](../ensemble/pki.md), [Storage](../ensemble/storage.md), [LLM Providers](../ensemble/llm-providers.md), [SSE](../ensemble/sse.md), [Development](../ensemble/devs.md), [Testing](../ensemble/tests.md), and [Evals](../ensemble/evals.md). Package-level entry points are [ensemble/README.md](../../ensemble/README.md), [ensemble/CONTRIBUTING.md](../../ensemble/CONTRIBUTING.md), and [ensemble/CHANGELOG.md](../../ensemble/CHANGELOG.md).

### Protocol documentation

- [Protocol README](../../protocol/README.md): Language packages, source ownership, directory map, generation, verification, and versioning.
- [Protocol Specification](../../protocol/docs/spec.md), [MCP](../../protocol/docs/mcp.md), [A2A](../../protocol/docs/a2a.md), and [Constants](../../protocol/docs/constants.md): Maintained wire and registry specifications.
- [Generated common API](../../protocol/docs/reference/api/g8e/common/v1/index.md), [compliance API](../../protocol/docs/reference/api/g8e/compliance/v1/index.md), [Operator API](../../protocol/docs/reference/api/g8e/operator/v1/index.md), and [pub/sub API](../../protocol/docs/reference/api/g8e/pubsub/v1/index.md): Generated protobuf field references.
- [Conformance](../../protocol/conformance/README.md), [Examples](../../protocol/examples/README.md), and [Python package](../../protocol/python/README.md): Cross-language contracts, runnable examples, and package usage.
- JSON schemas adjacent to MCP and A2A specifications, registries under `protocol/constants/`, models under `protocol/models/`, validation schemas under `protocol/schemas/`, and vectors under `protocol/vectors/` are machine-readable protocol documentation.

### References, diagrams, demos, and adapters

- [Glossary](../reference/glossary.md), [Compliance Alignment](../reference/compliance-alignment.md), [Compliance Evidence](../reference/compliance-evidence.md), and [FIPS 140-3](../reference/fips140-3.md): Terminology, scoped control/evidence claims, and cryptographic-module boundaries. `ksi-catalog.json` and `cosais-overlays.json` in the same directory are machine-readable references.
- Diagram sources cover the [system overview](../diagrams/flowchart-system-overview-lr.md), [Gateway fleet](../diagrams/graph-gateway-fleet-single-host-http-mtls.md), [Gateway services](../diagrams/graph-gateway-services.md), [Operator lifecycle](../diagrams/graph-operator-lifecycle.md), [L1-L5 Operator pipeline](../diagrams/graph-operator-pipeline-l1-l5.md), [50k system graph](../diagrams/graph-system-50k.md), and [principal-to-Operator sequence](../diagrams/sequence-principal-ensemble-gateway-operator-v3.md). [g8e-diagram.png](../diagrams/g8e-diagram.png) is the rendered image consumed by repository surfaces.
- [Demo index](../../demos/README.md), [Healthcare](../../demos/healthcare/README.md), [Finance](../../demos/finance/README.md), [DHS](../../demos/dhs/README.md), and [FedRAMP](../../demos/fedramp/README.md): Deployment-specific scenarios, topology, data, and verification boundaries.
- [Lattice Adapter](../../internal/adapters/lattice/README.md): Optional Anduril Lattice integration. Its imported protobuf source points to the separately owned third-party provenance README.

### Historical, evidence, and generated surfaces

- `docs/release_notes/` stores immutable per-release notes grouped by minor release. [CHANGELOG.md](../../CHANGELOG.md) is their current index. Compliance-evidence and offline-acceptance companions remain scoped to the named release and assessment.
- `docs/evidence/readme/candidates/` stores review candidates; `docs/evidence/readme/current/` stores the promoted checksum-bound snapshot consumed by the README generator. These JSON and JSONL artifacts are evidence, not narrative current-state documentation.
- `internal/services/gateway/docs/swagger.json` and `swagger.yaml` are generated OpenAPI outputs. `docs.go` embeds the JSON output in the binary.
- `website/` renders the generated root README into the public static site and packages its Cloudflare Worker.

## Cross-Linking Rules

- Every standalone component or subsystem README links to its canonical architecture or component index.
- Every component index lists every maintained document in that component’s documentation set.
- Every task guide links to the architecture or protocol reference needed to understand security and identity constraints.
- Every architecture document links to the task guides that operationalize it and to deeper references for schemas or algorithms.
- Every developer document links back to this guide when it defines documentation practice and to the Code Map when it describes repository ownership.
- The root README routes readers to product, deployment, architecture, protocol, component, compliance, developer, and support documentation. It does not duplicate every document title.
- Release notes link to exact artifacts for their release; current documents link to release notes only when historical context is necessary.
- Use relative links for repository content. Compute them from the directory containing the source document, not from the repository root.
- Link to a directory only when that directory is intentionally browsable and has an index. Prefer a concrete README or index document.
- Do not add links merely because a symbol is mentioned. Add links that establish ownership, navigation, prerequisites, or necessary context.

## Style and Structure

- Use present tense, active voice, and exact technical terminology.
- Write natural prose paragraphs without hard-wrapping source lines. Keep each list item and table cell on one source line.
- Use headings, lists, tables, and numbered procedures for navigation, not decorative formatting.
- Use inline backticks for commands, flags, configuration keys, routes, fields, symbols, file names, and repository paths.
- Use code blocks only for exact commands, configuration, requests, responses, or public API examples. Keep examples minimal, executable, and consistent with surrounding prerequisites.
- Use repository-relative source paths in developer documentation. Never publish machine-specific absolute paths, credentials, private endpoints, or private topology.
- Preserve existing front matter and metadata shape. Do not add redundant front matter solely to carry a title already present as a heading.
- Use diagrams when they clarify a trust boundary, topology, state transition, or sequence. Do not add decorative icons or emojis.
- Summarize a linked concept only far enough to establish context, then link to its canonical explanation.

## Prohibited Documentation Patterns

- Promotional adjectives, buzzwords, unsupported superlatives, or contrast-based marketing claims.
- Speculative features, unmerged behavior, unsupported roadmap statements, or retained code presented as active runtime behavior.
- Broad security claims that omit posture, ingress path, execution location, identity, evidence scope, trust input, or bypass boundary.
- Large copied endpoint tables, schemas, constants registries, algorithms, or package maps that already have a canonical owner.
- Line-number references to source code; use stable paths and symbols.
- Stale migration guidance in current-state documents. Keep historical behavior only in explicitly historical material.
- Manual edits to generated README, protobuf reference, or OpenAPI output without updating and running its source owner.
- Version or date changes used to imply review when the full document and related documentation were not audited.

## Validation Matrix

Run the checks owned by the changed surface. The repository does not currently define a general Markdown style or relative-link checker, so link and rendered-structure review remains an explicit manual step.

| Changed surface | Required validation |
| --- | --- |
| Handwritten Markdown only | Re-read every changed document end to end; resolve every changed relative link against its source directory; run any commands or focused tests whose behavior the prose documents |
| Root README template | `make readme`, `make readme-test`, `make readme-check`, and website checks when rendering changes |
| Public README evidence | Release-process projection, complete candidate review, digest-approved promotion, then README generator tests and drift check |
| Protobuf schema or comments | `make proto` plus affected Go, Python, TypeScript, and conformance checks |
| Swagger annotations | `make swagger-generate` plus relevant route/controller tests and generated-output review |
| Protocol constants, models, or catalogs | Affected contract/conformance tests and `make validate-doctrines` or `make validate-cosais` where applicable |
| Dashboard documentation | `make dashboard-test` and `make dashboard-lint` when claims depend on changed dashboard behavior |
| Ensemble or eval documentation | `make ensemble-test`, `make ensemble-lint`, `make evals-test`, or `make evals-lint` as applicable |
| Deployment documentation | Validate Compose configuration, help output, health checks, profiles, ports, and the narrow startup or command tests owned by the workflow |
| Release or evidence documentation | Follow [Release Process](release_process.md), including version synchronization and exact artifact verification |

The primary CI workflow regenerates protobuf and OpenAPI outputs, validates doctrine and COSAiS data, runs README generator tests and drift checks, and exercises component test jobs. CI does not replace the end-to-end factual and link audit.

## Completion Checklist

Before declaring a documentation change complete:

1. Read every changed document from first line to last line after the final edit.
2. Verify every behavioral, security, deployment, command, schema, and evidence claim against its owning current source.
3. Confirm all command names, flags, defaults, routes, methods, authentication classes, paths, ports, identities, statuses, and component names.
4. Confirm that generated files were updated only through their source and that machine-readable documentation remains synchronized.
5. Confirm that each changed document appears in this catalog or its linked component index and has useful inbound and outbound navigation.
6. Update all related current-state summaries and remove conflicting duplication.
7. Preserve historical and evidence scope; do not broaden claims beyond the cited artifacts.
8. Update `Last Updated` and `Version` only after the preceding audit is complete.
9. Run the owning validation commands and inspect their final output.
10. Perform a final structure, prose, metadata, and relative-link review without relying on CI to find documentation omissions.
