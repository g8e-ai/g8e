# g8e Documentation Guidelines

Internal authoring standards for g8e documentation. All contributors must follow these guidelines when creating or updating docs.

---

## Principles

- **Docs are code** — documentation is maintained with the same discipline as source code; stale or inaccurate docs are bugs
- **Authoritative, not aspirational** — document what the system does, not what it should do; never document planned or future behavior as if it exists
- **No redundancy** — each fact lives in exactly one place; cross-link rather than repeat
- **Sync on change** — any PR that changes behavior, APIs, constants, models, or configuration must update the relevant docs in the same change

---

## What Must Be Documented

| Change type | Required doc update |
|-------------|-------------------|
| New component or service | New component doc under `docs/components/`, entry in `docs/index.md` |
| New API endpoint or route | Update the relevant component doc |
| New wire-protocol constant, event, or status | Update `docs/components/` and `shared/` references |
| New shared model field | Update `shared/models/` description in component docs |
| New environment variable | Update `docs/developer.md` environment variable table |
| New test fixture, mock, or helper | Update `docs/testing.md` for the relevant component |
| New test type or test infrastructure change | Update `docs/testing.md` |
| New CLI command or flag | Update `docs/architecture/scripts.md` |
| New security mechanism | Update `docs/architecture/security.md` |
| New storage layer or data flow | Update `docs/architecture/storage.md` |
| Behavioral change to existing feature | Update the doc that owns that feature |
| Deprecation or removal | Remove the documentation entirely — never leave dead docs |

---

## File Locations

```
docs/
├── developer.md          # Quick start, infrastructure, code quality rules, project structure
├── testing.md            # Testing principles and component test guides
├── index.md              # Master index — every doc file must have an entry here
├── glossary.md           # All platform terminology, alphabetical
├── docs-guidelines.md    # This file
├── architecture/         # Cross-component internals: storage, security, AI control plane
├── components/           # Per-component reference: g8eo, g8ee, g8ed, g8es, g8ep
└── reference/            # External reference material (e.g. MCP protocol spec) and core platform principles
```

**Rules:**
- Component-specific behavior belongs in `docs/components/`.
- Cross-component data flows, protocols, and architectural decisions belong in `docs/architecture/`.
- External reference material belongs in `docs/reference/` — never modify files under `docs/reference/`.
- Every new doc file must be added to `docs/index.md`.
- `developer.md` component `#### Tests` subsections contain only code-quality rules (assertion discipline, model/constant usage, prohibited patterns). All test infrastructure — fixtures, mocks, helpers, cleanup, how to run, CI, test-runner containers — belongs exclusively in `testing.md`. Never duplicate these across the two files.

**Authoritative ownership — facts with a single home:**

| Fact | Authoritative location | Others cross-reference |
|------|----------------------|----------------------|
| Pub/sub channel names and wire format | `docs/components/g8es.md` | `g8ed.md`, `g8ee.md`, `g8eo.md`, `testing.md` |
| KV key namespace and patterns | `docs/components/g8es.md` | `g8ed.md`, `g8ee.md` |
| `G8eHttpContext` internal HTTP header full listing | `docs/components/g8ed.md` | `g8ee.md` cross-references; do not restate in other component docs |
| `X-Internal-Auth` shared secret (generation and discovery) | `docs/architecture/security.md` | `developer.md`, `g8es.md`, `g8ee.md`, `g8ed.md` |
| Heartbeat end-to-end flow | `docs/components/g8ed.md` | `g8eo.md`, `g8ee.md` |
| Shared constants and models (`shared/`) | `docs/developer.md` | `testing.md` |
| Universal code quality rules | `docs/developer.md` | do not restate in component docs |
| Component `#### Tests` code-quality rules (assertion discipline, model usage, anti-patterns) | `docs/developer.md` per-component section | cross-link to `testing.md` for infra |
| Testing principles, test-runner containers, CI workflows | `docs/testing.md` | do not restate in component docs or `developer.md` |
| Test infrastructure, fixtures, mocks, helpers, how to run tests | `docs/testing.md` per-component section | `developer.md` `#### Tests` subsections cross-link here |
| AI agents cross-component architecture (transport, pipeline) | `docs/architecture/ai_agents.md` | `docs/components/g8ee.md` |
| g8ee component internals (workflow modes, tools, LLM config, Sentinel, LFAA) | `docs/components/g8ee.md` | `docs/architecture/ai_agents.md` |
| g8ep container definition, volume mounts, SSH deployment | `docs/components/g8ep.md` | |
| Coverage goals per g8eo package | `docs/components/g8eo.md` | do not restate in `testing.md` |

---

## Document Structure

Every document must follow this structure:

```
# Title

One or two sentence summary of what this document covers and who it is for.

---

## Section

...
```

**Rules:**
- Start with an `H1` title followed immediately by a one- or two-sentence summary.
- Separate major sections with `---` horizontal rules.
- Use `H2` for top-level sections, `H3` for subsections — never skip levels.
- No table of contents — docs are short enough to not require one.

---

## Writing Style

- Write in the present tense — "g8eo sends a heartbeat every 30 seconds", not "g8eo will send".
- Use active voice — "g8ed validates the session", not "the session is validated by g8ed".
- Be direct and specific — avoid vague terms like "handles", "manages", "deals with".
- No filler phrases — "Note that", "Please be aware", "It is important to".
- No emojis anywhere in documentation.
- Refer to components by their canonical names: g8eo, g8ee, g8ed, g8es.

---

## Formatting

- Use fenced code blocks with a language tag for all code samples.
- Use tables for structured reference data (environment variables, error classes, status values, file locations).
- Use ASCII block diagrams for architecture and data-flow illustrations — match the style of existing diagrams in `docs/architecture/`.
- Bold is for emphasis on critical constraints only — not for decorative highlighting.
- Use bullet lists for unordered items; use numbered lists only when order matters.
- Inline code (backticks) for: file paths, environment variable names, tool  names, CLI commands, constant names.

---

## Code Samples

- All code samples must be accurate and runnable as written.
- Use the `./g8e` CLI for any platform commands — never raw `go test`, `pytest`, or `vitest`.
- Do not include placeholder values without clearly marking them (e.g. `<your-value>`).
- Keep samples minimal — show only what is necessary to illustrate the point.

---

## Cross-Linking

- Link to related docs rather than repeating content.
- Use relative paths for all internal links — never absolute URLs.
- When referencing a constant, model, or field defined in `shared/`, link to the relevant JSON file.
- `docs/index.md` is the entry point — every doc must be reachable from it.
- **Single source of truth enforcement:** when updating any doc, scan for content that properly belongs in a different doc and move it — do not leave duplicated content in both places. Add a cross-link from the source to the authoritative location after moving.

---

## Glossary

Any new platform-specific term introduced in a doc must also be added to `docs/glossary.md`. Terms in docs should match glossary definitions exactly — if the definition needs updating, update the glossary first.

---

## What Not to Document

- **Implementation details that belong in code comments** — if it only matters to the author of a function, it belongs in the source, not in docs.
- **Transient state** — do not document in-flight behavior, retry internals, or timing assumptions that are subject to change.
- **Future plans** — docs describe the current system only.
- **Duplicates of `developer.md` or `testing.md`** — do not restate the universal code quality rules or testing principles in component docs.
- **Test infrastructure details in `developer.md`** — fixture names, mock constructors, helper functions, how to run tests, and cleanup patterns belong in `testing.md`; `developer.md` component `#### Tests` subsections must only contain code-quality rules and cross-link to `testing.md` for everything else.
