# AI Documentation Agent Guide

This document defines the instructions, stylistic rules, and terminology conventions for any AI agent tasked with updating or creating documentation for the g8e platform. Adherence to these guidelines ensures a professional, technically accurate, and consistent documentation footprint.

## 1. Tone and Style Standards

All documentation must maintain a professional systems-engineering tone. Avoid promotional vocabulary, marketing-oriented text structures, and informal formatting.

### Linguistic Guardrails
- **Dry and Factual**: State technical behaviors directly. Do not use promotional adjectives, hyperbolic claims, or industry buzzwords (such as "revolutionary", "seamless", "powerful", or "cutting-edge").
- **Strict Present Tense**: Document existing platform behaviors in the present tense (for example, "The gateway validates the incoming envelope," rather than "The gateway will validate the incoming envelope").
- **No Contrast-y Comparison Tropes**: Frame platform capabilities standalone. Do not use contrast-y marketing tropes (such as "unlike traditional frameworks" or "not just a tool, but a solution").
- **Technical Precision**: Employ exact cryptography, distributed systems, and network security terminology. Refer to specific files, structures, and schemas rather than high-level abstractions.

### Formatting and Punctuation
- **No Em-Dashes**: Do not use em-dashes (—) under any circumstances. Construct compound clauses and modifiers using commas, semicolons, or single hyphens.
- **No Emojis**: Maintain a standard engineering publication appearance. Do not use emojis, icons, or descriptive illustrations in headers, bullet points, or body text.
- **Codebase References (Portable)**: When referencing files, line ranges, or symbols in the codebase, use repository-relative paths (e.g., `internal/services/governance/l1_doctrine.go`). Do not use absolute filesystem paths (e.g., `/home/bob/...`) as this breaks portability.
- **Documentation Links (Relative Only)**: All markdown links to other documentation files MUST use relative paths from the current document's location (e.g., `[Gateway](./gateway.md)` or `[Architecture](../architecture/gateway.md)`). Never use absolute paths.

---

## 2. Terminology Conventions

Maintain absolute consistency with the authoritative system nomenclature used in the codebase. Never refer to legacy, deprecated, or superseded components in user-facing or developer guides.

---

## 3. Source-of-Truth Hierarchy

Before making any documentation updates, you must query and verify the underlying implementation in the codebase. Never guess or speculate on system behavior. Trace system capabilities in this precise order:

1. **Protobuf Schemas**: Read schema definitions to verify fields, types, and validation options.
2. **Constants Registries**: Read constants files to verify endpoint paths, collection names, and identifiers.
3. **Core Service Implementations**: Analyze service implementations to confirm current business logic, error propagation, and security gates.

---

## 4. The Five-Layer Interlock Sequence

When documenting any security, execution, or transactional pipeline, you must trace the validation sequence sequentially across the five core service layers:

- **L1 Doctrine**: Technical Bedrock (Hard Gates) code pattern matching and threat analysis.
- **L2 Consensus**: Multi-agent consensus signature verification using Ed25519 cryptography.
- **L3 Notary**: Human-in-the-loop authorization (utilizing WebAuthn or cryptographically signed CLI proofs).
- **L4 Warden**: Pre-dispatch verification gating (validating signatures, replay prevention, expiry, nonces, and state Merkle root).
- **L5 Actuator**: Isolated boundary tool dispatch (via MCP/A2A) and signed receipt production.

---

## 5. Documentation Separation Principle

The repository maintains a strict separation between two documentation domains:

- **Protocol Reference (`protocol/docs/`)**: Contains protocol wire specifications, schema definitions, constant references, and generated API docs. These documents describe the protocol contract independent of any platform implementation. A protocol library consumer should not need to navigate the platform `docs/` tree to find the protocol specification.
- **Platform Documentation (`docs/`)**: Contains user guides, architecture overviews, compliance reports, developer guides, and position papers. These documents describe how the g8e platform implements and uses the protocol.

When creating or updating documentation, place it in the correct domain. Protocol specs, schemas, and constant references belong in `protocol/docs/`. Platform guides, architecture docs, and compliance material belong in `docs/`. Cross-references between the two domains must use relative paths (e.g., `../../protocol/docs/spec.md` from within `docs/`).

---

## 6. Documentation Lifecycle Procedures

When modifying, extending, or creating documentation within this repository, follow this execution protocol:

1. **Analyze Current Implementation**: Trace the execution path through the codebase starting from the protocol boundaries to verify the accuracy of the proposed documentation updates.
2. **Remove Outdated Information**: Fully delete any mentions of deprecated components, obsolete architectural layouts, or legacy terminology.
3. **Align with active codebase**: Do not document feature ideas, speculative roadmaps, or unmerged code. Public documentation must reflect the live repository state.
4. **Compile and Format**: Verify that all modified markdown files adhere to markdown syntax, contain zero emojis, utilize present tense, use only commas/semicolons/single-hyphens rather than em-dashes, and use *relative path* citations.
