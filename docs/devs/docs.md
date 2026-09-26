---
doc_id: docs
title: Documentation Guide
audience: maintainers and coding agents
status: current
last_updated: 2026-09-26
version: v2.2.0
owners:
  - docs/
  - protocol/docs/
  - README.md
  - buf.gen.yaml
  - Makefile
related:
  - docs/devs/devs.md
  - docs/devs/codemap.md
  - docs/devs/tests.md
  - docs/devs/release_process.md
  - docs/devs/troubleshooting.md
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
| INV-DOC-GEN-01 | Generated protobuf references in `protocol/docs/reference/` MUST change only through edits to `.proto` files in `protocol/proto/g8e/` followed by `make proto`. MUST NOT hand-edit generated protobuf output. |
| INV-DOC-GEN-02 | Gateway OpenAPI specifications (`internal/services/gateway/docs/swagger.json` and `swagger.yaml`) MUST change only through Go Swagger annotations followed by `make swagger-generate`. |
| INV-DOC-GEN-03 | Machine-readable protocol constants and schemas under `protocol/constants/`, `protocol/models/`, and `protocol/schemas/` MUST remain synchronized with their Go, Python, and TypeScript mirrors via owning validation commands. |

### Style and structure (`INV-DOC-STYLE`)

| ID | Rule |
| --- | --- |
| INV-DOC-STYLE-01 | Prose MUST use present tense, active voice, and exact technical terminology. Source lines MUST NOT be hard-wrapped. |
| INV-DOC-STYLE-02 | Code references MUST use repository-relative paths and stable symbol names. Line-number references and machine-absolute paths are prohibited. |
| INV-DOC-STYLE-03 | Relative links MUST resolve from the directory containing the document. Link syntax MUST use `[text](target.md)`. |

## Owned surfaces

| Claim | Path | Verify |
| --- | --- | --- |
| Dev docs format | `docs/devs/` | YAML front matter and required H2 section order |
| Documentation catalog | `docs/devs/docs.md` | Authoritative inventory of all first-party docs |
| Protobuf API references | `protocol/proto/g8e/`, `protocol/docs/reference/` | `make proto` |
| Gateway OpenAPI | `internal/services/gateway/docs/`, Go Swagger annotations | `make swagger-generate` |
| Protocol constants | `protocol/constants/`, `internal/constants/` | `make validate-doctrines`, `make validate-cosais` |

## Procedures

### End-to-End Audit Workflow

1. Read the complete document from beginning to end.
2. Classify document purpose (guide, architecture, reference, developer, protocol, historical).
3. Inventory every claim (commands, flags, routes, defaults, ports, paths, contracts).
4. Trace each claim to its primary owning source in the repository.
5. Inspect related documentation to update cross-links and eliminate conflicting duplicates.
6. Refresh generated outputs through source owners (`make proto`, `make swagger-generate`).
7. Review revised document end-to-end for coherence, relative links, and clean formatting.
8. Update `last_updated` and `version` metadata last.
9. Run owning validation commands (`./g8e test lint`, component tests, or schema validators).

### Refresh Generated Documentation

```bash
# Protobuf references, Go/Python/Node code, downstream lockfiles
make proto

# Gateway Swagger / OpenAPI JSON and YAML
make swagger-generate

# Validate doctrine and COSAiS JSON catalogs
make validate-doctrines
make validate-cosais
```

## Anti-patterns

- Hand-editing generated protobuf references or Swagger JSON without updating sources (INV-DOC-GEN-01, INV-DOC-GEN-02).
- Updating `last_updated` or `version` without auditing the full document (INV-DOC-FMT-02).
- Hard-wrapping source prose lines or embedding source code line numbers (INV-DOC-STYLE-01, INV-DOC-STYLE-02).
- Copying full CLI flag inventories or large schema tables instead of linking (INV-DOC-AUTH-02, INV-DOC-AUTH-03).
- Claiming third-party certifications or unqualified security guarantees (INV-DOC-AUTH-04).

## Links out

- [Developer Guidelines](devs.md): coding invariants and repository standards.
- [Code Map](codemap.md): package and runtime ownership maps.
- [Testing Guide](tests.md): test execution tiers and testing invariants.
- [Release Process](release_process.md): versioning, compliance bundles, and release workflow.
- [Troubleshooting](troubleshooting.md): diagnostics and recovery procedures.
