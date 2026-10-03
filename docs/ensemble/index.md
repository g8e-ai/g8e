---
doc_id: ensemble_index
title: g8ee Ensemble Documentation Index
audience: developers, operators, and coding agents
status: current
last_updated: 2026-09-28
version: v2.2.3
owners:
  - docs/ensemble/
  - docs/ensemble/index.md
related:
  - architecture.md
  - agents.md
  - governance.md
  - protocol.md
  - devs.md
  - tests.md
  - ../architecture/ensemble.md
when_to_read: Navigating g8ee documentation topics, finding ownership boundaries for ensemble components, or understanding the documentation structure and topic relationships.
do_not_use_for:
  - Ensemble component architecture and request flow (architecture.md)
  - Agent personas and Tribunal reasoning (agents.md)
  - Five-layer verification and postures (governance.md)
  - Wire protocols and Gateway integration (protocol.md)
  - Platform-wide architecture and governance (../architecture/)
---

# g8ee Ensemble Documentation Index

## Purpose

Provides a navigational index to g8e Agentic Ensemble (`g8ee`) documentation across setup, architecture, governance, development, and testing topics. Establishes ownership boundaries and directs readers to canonical sources for specific subject areas.

## Quick index

- [Purpose](#purpose)
- [Quick index](#quick-index)
- [Invariants](#invariants)
- [Owned surfaces](#owned-surfaces)
- [Procedures](#procedures)
- [Anti-patterns](#anti-patterns)
- [Links out](#links-out)

## Invariants

Ids are stable. Append the next free number in each group; do not renumber.

### Documentation Navigation (`INV-ENS-IDX-NAV`)

| ID | Rule |
| --- | --- |
| INV-ENS-IDX-NAV-01 | The ensemble index MUST catalog all maintained documents under `docs/ensemble/`. Each entry MUST include a brief one-line description of scope and target audience. |
| INV-ENS-IDX-NAV-02 | Documents under `docs/ensemble/` own ensemble-specific behavior: agents, governance application (pre-L1), protocol surfaces, development practices, and testing. Platform-wide architecture, five-layer verification, and third-party integration belong in `docs/architecture/`. |
| INV-ENS-IDX-NAV-03 | Each document listed in the index MUST be maintained, audit-current, and verifiable against ensemble source code at `ensemble/`. Stale or placeholder entries are prohibited. |

### Related Documentation (`INV-ENS-IDX-REL`)

| ID | Rule |
| --- | --- |
| INV-ENS-IDX-REL-01 | Platform-level documents under `docs/architecture/` establish the governance boundary, five-layer verification, Gateway and Operator architecture, authentication, and networking. Ensemble documents reference platform architecture for context but do not replicate it. |
| INV-ENS-IDX-REL-02 | First-party integration guides under `docs/guides/` describe deployment patterns, Docker Compose stacks, and platform setup. They cross-reference ensemble documentation for component-specific details. |

## Owned surfaces

| Claim | Path | Verify |
| --- | --- | --- |
| Ensemble documentation index | `docs/ensemble/index.md` | All listed documents exist and have YAML front matter |
| Ensemble-owned topics | `docs/ensemble/*.md` | doc_id matches `ensemble_*` or specific feature scope; `version` matches VERSION |
| Related platform docs | `docs/architecture/ensemble.md` | Cross-links to `docs/ensemble/` for component details |

## Procedures

### Maintaining the Documentation Index

When adding a new ensemble document:

1. Write the document in `docs/ensemble/` with YAML front matter following [Documentation Guide](../devs/docs.md) format.
2. Set `doc_id` to `ensemble_` followed by a stable slug (e.g., `ensemble_caching`, `ensemble_migrations`).
3. Add an entry to the table below with a description, path, and brief explanation of when to read it.
4. Update this document's metadata (`last_updated`, `version`) only after verifying all entries.
5. Run `./g8e test lint` to verify formatting and relative links.

### Verifying Documentation Accuracy

When auditing ensemble documentation:

1. Read the complete document from beginning to end.
2. Classify its purpose (guide, reference, architecture, developer, protocol, historical).
3. Inventory every claim (endpoints, configuration, proto messages, code paths).
4. Trace each claim to its owning source in `ensemble/`, `internal/`, or `protocol/` directories.
5. Update document metadata and report findings through a pull request to `main`.

## Anti-patterns

- Listing documents without YAML front matter or outdated `version` metadata in the index.
- Duplicating platform governance or architecture content from `docs/architecture/` in ensemble-specific documents.
- Adding entries for aspirational or placeholder documents that lack implementation.
- Hand-editing generated protobuf references without regenerating via `make proto`.
- Hard-coding line numbers or absolute paths in cross-references instead of repository-relative paths.

## Links out

### Ensemble Documentation

| Document | Description |
| --- | --- |
| [Getting Started](getting-started.md) | Prerequisites, installation, configuration, and unified stack setup |
| [Architecture](architecture.md) | Component lifecycle, request flow, trust boundaries, and service dependencies |
| [Governance](governance.md) | Application-level controls, Tribunal voting, command risk analysis, and verification postures |
| [Agents](agents.md) | Agent personas, Tribunal composition, reasoning loop, and prompt templating |
| [Protocol](protocol.md) | GovernanceEnvelope schema, wire contracts, and Gateway integration points |
| [Prompts](prompts.md) | Prompt architecture, templating system, and configuration |
| [Thinking](thinking.md) | L2 consensus voting, provider reasoning, and thought signatures |
| [PKI & Trust](pki.md) | Client certificates, SPIFFE identity, trust bundles, and workload enrollment |
| [Storage](storage.md) | Storage tiers, cache strategies, state ownership, and persistence |
| [LLM Providers](llm-providers.md) | Provider implementations, model selection, and configuration |
| [Server-Sent Events (SSE)](sse.md) | SSE streaming pipeline, event delivery, and real-time subscriptions |
| [Development](devs.md) | Dev setup, local testing, code style, and contribution guidelines |
| [Testing](tests.md) | Test framework, fixture patterns, integration tests, and CI scope |
| [Evals](evals.md) | Evaluation programs, Observer integration, and platform evidence collection |
| [Decision Providers](decision-providers.md) | Decision provider implementations and custom provider registration |

### Platform Documentation

| Document | Description |
| --- | --- |
| [Platform Overview](../architecture/overview.md) | Three-component g8e platform: Gateway, Operator, and Ensemble roles |
| [Ensemble (Platform View)](../architecture/ensemble.md) | Platform-level ensemble architecture, trust boundaries, and governance interlock |
| [Governance Pipeline](../architecture/governance.md) | Five-layer verification (L1-L5), postures, and transaction flow |
| [Governance Gateway](../architecture/gateway.md) | Gateway architecture, PKI authority, policy decision point, and routing |
| [Governed Operator](../architecture/operator.md) | Operator architecture, L4 Warden, L5 Actuator, and execution boundaries |
| [Protocol Reference](../architecture/protocol.md) | Canonical wire contracts, GovernanceEnvelope, and SPIFFE identifiers |
| [Authentication & Authorization](../architecture/auth.md) | mTLS, WebAuthn, SPIFFE workload identity, and trust bundles |
| [Agents and AI Integration](../architecture/agents.md) | Supported agent types, integration boundaries, and governance limits |
| [Networking](../architecture/network.md) | Service topology, port allocation, and network security |
| [SSE Streaming](../architecture/sse.md) | Gateway-side SSE ingestion, filtering, and consumer endpoints |
| [Evaluations](../architecture/evals.md) | Evaluation programs, Observer role, evidence collection, and verification |
| [Model Provenance](../architecture/model-provenance.md) | Weight attestation, storage-side verification, and chain of custody |
| [Getting Started Guide](../guides/getting_started.md) | Platform installation, deployment, and quick start |
| [Unified Docker Stack](../guides/unified_stack.md) | Docker Compose deployment and unified stack management |
| [Console](../architecture/console.md) | First-party browser interface and event consumption |
| [Documentation Guide](../devs/docs.md) | Repository-wide documentation standards, ownership, and audit procedures |
