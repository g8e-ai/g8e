# AI Documentation Agent Guide

Guidelines for any AI agent creating or updating g8e documentation. The codebase is the source of truth; documentation describes what the system does, not how.

## Always

- Use present tense for existing behaviors.
- Use exact cryptography, distributed systems, and network security terminology.
- Name specific components and interfaces at the architecture level.
- Use repository-relative paths for codebase references: `internal/services/governance/l1_doctrine.go`.
- Use relative paths for documentation links: `[Gateway](./gateway.md)`, `[Architecture](../architecture/gateway.md)`.
- Use short inline backticks for configuration keys, CLI flags, or file names.
- Use headers, bullet lists, and numbered steps for scannable structure.
- Keep paragraphs to 3 to 5 sentences. If a paragraph exceeds 5 lines, restructure it.
- Summarize linked concepts in one sentence and link rather than duplicating.
- Link only critical files (core service entry points, key interface definitions).
- Verify behavior against the codebase before writing. Trace in this order:
  1. Protobuf schemas (fields, types, validation).
  2. Constants registries (endpoint paths, collection names, identifiers).
  3. Core service implementations (business logic, error propagation, security gates).
- Place protocol specs in `protocol/docs/`. Place platform guides in `docs/`.
- Cross-reference between domains with relative paths: `../../protocol/docs/spec.md`.
- Document the five-layer interlock sequence when covering security or execution pipelines:
  - **L1 Doctrine**: Hard gates, code pattern matching, threat analysis.
  - **L2 Consensus**: Multi-agent consensus signature verification (Ed25519).
  - **L3 Notary**: Human-in-the-loop authorization (WebAuthn or signed CLI proofs).
  - **L4 Warden**: Pre-dispatch verification (signatures, replay prevention, expiry, nonces, Merkle root).
  - **L5 Actuator**: Isolated tool dispatch (MCP/A2A) and signed receipt production.
- Remove deprecated, obsolete, or legacy references during updates.
- Reflect only the live repository state.

## Never

- Use promotional adjectives or buzzwords ("revolutionary", "seamless", "powerful", "cutting-edge").
- Use contrast-y marketing tropes ("unlike traditional frameworks", "not just a tool, but a solution").
- Use em-dashes. Use commas, semicolons, or single hyphens instead.
- Use emojis, icons, or descriptive illustrations.
- Use absolute filesystem paths (`/home/bob/...`).
- Embed code snippets, function signatures, struct definitions, or multi-line code examples.
- Name private methods, internal helper types, or implementation details unless part of a public API.
- Document feature ideas, speculative roadmaps, or unmerged code.
- Guess or speculate on system behavior.
- Repeat information across sections. Cross-reference with a link instead.
- Link every file or function mentioned.

## Examples

**Present tense, dry tone:**
> The gateway validates the incoming envelope.

**Relative documentation link:**
> See [Gateway Architecture](../architecture/gateway.md) for transport details.

**Repository-relative codebase reference:**
> The doctrine service entry point lives in `internal/services/governance/l1_doctrine.go`.

**Inline configuration key (allowed):**
> Set `max_envelopes` to control the dispatch buffer size.
