# g8e Release Process — Version & Documentation Updates

This document is the definitive checklist for updating all version-bearing files and documentation when bumping a g8e release. The protocol (Go + Python) and the platform binary share the same version number — there are no separate protocol releases.

> **`make release` handles version syncing, tests, and builds.** It does NOT touch git. After reviewing the changes, commit them and run `make release-tag` to tag and push. See [Release Workflow](#release-workflow).

## How to Use This Document

1. Determine the new version number (see [Versioning Rules](#versioning-rules))
2. Set `VERSION` to the new version (the single source of truth)
3. Run `make release` — this auto-syncs `pyproject.toml` and `__init__.py`, runs protocol tests, and builds binaries
4. Update `CHANGELOG.md` and create release notes (see [Manual Updates](#manual-updates))
5. Review release content for [documentation accuracy](#content-accuracy-review) — protocol specs, architecture docs, and guides must reflect any code refactors or behavioral changes shipped in this release
6. Run the [Verification](#verification) section to catch any missed files
7. Commit all changes and run `make release-tag` to tag and push

---

## Versioning Rules

Follow [Semantic Versioning](https://semver.org/spec/v2.0.0.html):

- **MAJOR** (X.0.0): Breaking changes that require user action
- **MINOR** (x.Y.0): New features, backward-compatible changes
- **PATCH** (x.y.Z): Bug fixes, backward-compatible changes

Check the current version before starting:

```bash
cat VERSION
```

Throughout this document, **`vX.Y.Z`** refers to the new release version (e.g., `v1.3.1`), and **`YYYY-MM-DD`** refers to the release date. Note the two version string conventions in use:

- **With `v` prefix** (`vX.Y.Z`): `VERSION`, all doc `Version:` headers, git tags
- **Without `v` prefix** (`X.Y.Z`): `CHANGELOG.md`, Python package version, `**Document Version:**` headers

---

## Version-Bearing Files

The following files contain version strings. `VERSION` is the single source of truth — `make release` auto-syncs all derived files from it.

### A. Core Version Files

| # | File | How It's Updated | Format |
|---|------|-----------------|--------|
| 1 | `VERSION` | **Manual** — set to new version | `vX.Y.Z\n` (with trailing newline, no trailing spaces) |
| 2 | `CHANGELOG.md` | **Manual** — add new section below `## [Unreleased]` | `## [X.Y.Z] - YYYY-MM-DD` (no `v` prefix) |
| 3 | `protocol/python/pyproject.toml` | **Auto** — `make release` syncs from `VERSION` | `version = "X.Y.Z"` (no `v` prefix) |
| 4 | `protocol/python/g8e/__init__.py` | **Auto** — `make release` syncs from `VERSION` | `__version__ = "X.Y.Z"` (no `v` prefix) |

> ⚠️ Items 3 and 4 are auto-synced by `make release` and verified by `make release-tag` and CI. A mismatch will fail the release.

#### CHANGELOG.md Template

Use the [Keep a Changelog](https://keepachangelog.com/en/1.0.0/) sections. Only include the subsections that apply to the release — `Removed` and `Deprecated` are part of the standard and should be used when relevant (e.g., v1.3.1 was a `Removed`-only cleanup release).

```markdown
## [X.Y.Z] - YYYY-MM-DD

### Overview

[Brief summary of the release — 2-3 sentences highlighting major changes]

### Breaking Changes

* **Change title** - Description of breaking change and migration path

### Added

* **Feature title** - Description of new feature

### Changed

* **Change title** - Description of modification

### Deprecated

* **Item title** - What is deprecated and the recommended replacement

### Removed

* **Item title** - What was removed and why

### Fixed

* **Bug title** - Description of fix

### Security

* **Security title** - Description of security improvement
```

### B. Release Notes File

Create a new release notes file for every release.

| # | File | Action |
|---|------|--------|
| 5 | `docs/release_notes/vX.Y.x/vX.Y.Z.md` | Create new file in the minor-version subdirectory (e.g., `v1.3.x/v1.3.1.md`) |

The release notes file should mirror the CHANGELOG entry but can be more detailed. Include the version header and date, an Overview, all applicable Keep-a-Changelog sections, and any additional context, examples, or links to relevant documentation. Past release notes are immutable — never edit historical entries.

#### Release Notes Template

Use the same `## [X.Y.Z] - YYYY-MM-DD` header as the CHANGELOG (no `v` prefix in the bracket). Only include the subsections that apply to the release — most releases use 2–4 of these, not all of them.

```markdown
## [X.Y.Z] - YYYY-MM-DD

### Overview

[Brief summary of the release — 2-5 sentences highlighting major changes, themes, and motivation]

### Breaking Changes

* **Change title** — Description of breaking change and migration path

### Added

* **Feature title** — Description of new feature

### Changed

* **Change title** — Description of modification

### Deprecated

* **Item title** — What is deprecated and the recommended replacement

### Removed

* **Item title** — What was removed and why

### Fixed

* **Bug title** — Description of fix

### Security

* **Security title** — Description of security improvement

### Deferred

* **Item title** — What was considered but deferred, and why

### Tests

* Description of significant test coverage additions or refactors
```

**Conventions observed across existing release notes:**

- **Bullet format**: `* **Item title** — Description` using an em-dash (`—`) separator between the bold title and description. Sub-bullets use `- ` for nested detail.
- **Section selection**: Only include sections that have content. Common combinations: small fix releases use just `Overview` + `Fixed`; feature releases use `Added` + `Changed` + `Fixed`; cleanup releases use `Removed` + `Changed` + `Fixed`.
- **Optional sections**: `Deferred`, `Tests`, and `Documentation` are not part of the Keep-a-Changelog standard but have been used in past release notes when those categories carry significant content. Use them when appropriate.
- **Detail level**: Release notes can be more verbose than the CHANGELOG entry — include file paths, function names, and links to relevant docs where helpful.
- **Trailing separator**: Some release notes end with a `---` horizontal rule. This is optional.

### C. Documentation with Version / Date Headers

Every markdown file below carries a version and/or `Last Updated` header near the top. **Only update headers in docs that were actually reviewed or modified as part of this release.** Do not blanket-bump all headers at release time — a doc whose content hasn't changed should not get a new version stamp.

To determine which docs need header updates, diff the release range:

```bash
git diff --name-only <previous-tag>..HEAD -- docs/ protocol/docs/
```

Any file in the output that carries a version/date header should have that header updated to the new version and release date. Files not in the output are left as-is.

> **Header formats vary** — they are not uniform across the repo. The [Verification](#verification) grep is written to catch all of them, but when editing by hand watch for:
>
> | Format | Example | Files |
> |--------|---------|-------|
> | Plain | `Version: vX.Y.Z` / `Last Updated: YYYY-MM-DD` | most architecture, guide, reference, and protocol docs |
> | Bold | `**Version:** vX.Y.Z` / `**Last Updated:** YYYY-MM-DD` | (none currently) |
> | Document Version (no `v`) | `**Document Version:** X.Y.Z` | `docs/reference/compliance-alignment.md` |
> | Date only (no version line) | `Last Updated: YYYY-MM-DD` | `protocol/docs/a2a.md`, `protocol/docs/mcp.md` |

#### Architecture Docs (`docs/architecture/`) — 8 files

| # | File |
|---|------|
| 6 | `docs/architecture/auth.md` |
| 7 | `docs/architecture/encryption.md` |
| 8 | `docs/architecture/gateway.md` |
| 9 | `docs/architecture/governance.md` |
| 10 | `docs/architecture/network.md` |
| 11 | `docs/architecture/operator.md` |
| 12 | `docs/architecture/sse.md` |
| 13 | `docs/architecture/storage.md` |

#### Guide Docs (`docs/guides/`) — 9 files

| # | File | Note |
|---|------|------|
| 14 | `docs/guides/air_gap.md` | |
| 15 | `docs/guides/build_apps.md` | |
| 16 | `docs/guides/build_gateway.md` | |
| 17 | `docs/guides/build_operator.md` | |
| 18 | `docs/guides/cli.md` | |
| 19 | `docs/guides/connect_apps_to_gateway.md` | |
| 20 | `docs/guides/connect_operator_to_gateway.md` | |
| 21 | `docs/guides/docker_gateway.md` | |
| 22 | `docs/guides/getting_started.md` | |

#### Reference Docs (`docs/reference/`) — 2 files

| # | File | Note |
|---|------|------|
| 23 | `docs/reference/glossary.md` | plain `Version:` header |
| 24 | `docs/reference/compliance-alignment.md` | `**Document Version:**` (no `v` prefix) |

#### Protocol Docs (`protocol/docs/`) — 3 files

| # | File | Note |
|---|------|------|
| 25 | `protocol/docs/spec.md` | has both `Version:` and `Last Updated:` |
| 26 | `protocol/docs/a2a.md` | `Last Updated:` only — update the date |
| 27 | `protocol/docs/mcp.md` | `Last Updated:` only — update the date |

#### Update Pattern for Each Modified Doc

For plain-format files, update the two header lines:

```diff
-Last Updated: 2026-06-24
-Version: v1.3.0
+Last Updated: YYYY-MM-DD
+Version: vX.Y.Z
```

For bold-format and date-only files, update whichever of `Version`, `Document Version`, and `Last Updated` lines are present, preserving the existing markdown styling.

> **Only update docs you actually reviewed.** If a doc's content didn't change in this release, leave its headers at the previous version. This keeps version stamps meaningful — they reflect the last time a human reviewed and updated the doc, not the last release tag.

### D. Docs Without Version Headers (No Version Update Required)

The following doc directories intentionally do **not** carry release `Version:` headers and do **not** need version bumps on release. They may still need *content* updates if the release changes their subject matter (see [Content Accuracy Review](#content-accuracy-review)):

- `docs/core/` — Position papers, about page
- `docs/devs/` — Developer documentation (including this file). Note: some devs docs (e.g., `docs/devs/tests.md`) carry a `Last Updated:` date that tracks the doc's own content changes, **not** the release — only bump it if you changed the doc.
- `docs/diagrams/` — Mermaid diagrams and flowcharts
- `docs/release_notes/` — Historical release notes (past entries are immutable)
- `demos/` — Demo configurations and doctrine files
- `README.md` — Uses a dynamic GitHub badge for latest release; no hardcoded version

### E. Go Protocol Module

| File | Notes |
|------|-------|
| `protocol/go.mod` | No version field to update. The Go module version is derived from git tags (`protocol/vX.Y.Z`), created by `make release-tag`. |

### F. Build & CI Files (No Version Update Required)

The following files read the version dynamically from `VERSION` at build time and do **not** require manual updates:

- `Makefile` — Reads `VERSION` via `$(shell cat VERSION)`; `make release` syncs Python files; `make release-tag` tags and pushes
- `Dockerfile` / `Dockerfile.operator` — Receive version as build arg
- `docker-compose.yml` — References build context, not version
- `.github/workflows/*.yml` — Triggered by git tags, no hardcoded version. CI includes a version sync check that fails if Python files don't match `VERSION`.

---

## Content Accuracy Review

Version bumps are not enough. A release frequently includes code refactors, removed/renamed commands, changed endpoints, or new behavior that makes documentation **factually stale**. Before finishing, review the documentation against the actual code changes in this release.

### When to Update Documentation

**Only update a doc when one of these conditions is true:**

1. **Inaccuracy** — The doc describes something that is no longer true (a renamed command, a removed endpoint, a changed default, a corrected behavior). Fix the inaccurate prose so it matches the code.
2. **Missing feature** — The release adds a user-visible feature, command, endpoint, or config option that has no documentation at all. Add the missing documentation.
3. **Stale reference** — The doc references something that was removed or renamed in this release (e.g., a deprecated alias, a deleted route, a renamed constant). Remove or update the reference.

**Do NOT update a doc when:**

- The doc is already accurate — even if the underlying code was refactored internally, if the user-facing behavior and interface are unchanged, the doc is fine as-is.
- The doc wasn't touched by any code change in this release — leave its version header and content alone.
- You're tempted to "improve" prose that isn't wrong — cosmetic rewrites are not part of the release process.

> **Principle: Fix what's broken, document what's missing, leave the rest alone.** The goal is accuracy, not thoroughness. A doc that correctly describes the current behavior is done — don't rewrite it just because you read it.

### What to Review

Review the following documentation areas against the actual code changes in this release. For each area, only make edits if you find an inaccuracy or a missing feature per the rules above.

- **Protocol specs** (`protocol/docs/spec.md`, `a2a.md`, `mcp.md`) — If the protocol surface changed (endpoints, JSON-RPC methods, message shapes, auth flows), update the spec prose, not just the date. The Python (`protocol/python/`) and Go (`protocol/proto/`, generated code) bindings must agree with what the spec documents.
- **Architecture docs** (`docs/architecture/`) — If components, data flows, or security boundaries were refactored, reconcile the prose and diagrams.
- **Guides** (`docs/guides/`) — If CLI commands, flags, env vars, or setup steps changed, update the affected guides and any embedded command examples.
- **Glossary / compliance** (`docs/reference/`) — If terminology or control mappings changed, reconcile them.
- **CHANGELOG / release notes** — Ensure every user-visible change (added, changed, removed, deprecated, fixed, security) is captured.

### How to Find Stale References

> Tip: diff the release range and scan for removed/renamed identifiers, CLI commands, routes, and config keys, then grep the docs for each to find stale references. Anything removed in code (e.g., a deprecated alias or endpoint) should have no lingering documentation telling users to use it.

```bash
# List all files changed in the release
git diff --name-only <prev-tag>..HEAD

# Search docs for identifiers that were removed or renamed
grep -rnE 'oldCommandName|OldEndpointPath|OLD_CONSTANT' docs/ protocol/docs/ --include='*.md'
```

---

## Manual Updates

`make release` handles version syncing, tests, and builds automatically. The following must still be done manually:

- [ ] **1. `VERSION`** — Set to `vX.Y.Z`
- [ ] **2. `make release`** — Auto-syncs `pyproject.toml` + `__init__.py`, runs protocol tests, builds binaries
- [ ] **3. `CHANGELOG.md`** — Add `## [X.Y.Z] - YYYY-MM-DD` section (no `v` prefix)
- [ ] **4. `docs/release_notes/vX.Y.x/vX.Y.Z.md`** — Create new release notes file
- [ ] **5. Documentation headers** — Update version/date headers **only** in docs that were actually reviewed or modified in this release (use `git diff --name-only <prev-tag>..HEAD -- docs/ protocol/docs/` to identify them). Do not blanket-bump all headers. See [Section C](#c-documentation-with-version--date-headers) for the full list of files that carry headers.
- [ ] **6. Content review** — Complete the [Content Accuracy Review](#content-accuracy-review). Only update docs to fix inaccuracies or document missing features — do not rewrite accurate docs.
- [ ] **7. Commit and tag** — `git add -A && git commit -m "release: vX.Y.Z"` then `make release-tag`

**Only 2 files need manual version edits** (VERSION + CHANGELOG). The Python package files are auto-synced. Release notes and doc header updates are content-driven, not mechanical version bumps.

---

## Verification

After making all updates, run these checks to catch any missed files. These are all read-only.

```bash
RELEASE_VERSION=$(cat VERSION)        # e.g. v1.3.1
RELEASE_NUM=${RELEASE_VERSION#v}      # e.g. 1.3.1
RELEASE_DATE="YYYY-MM-DD"             # set to the release date

# 1. Verify VERSION file
cat VERSION

# 2. Verify CHANGELOG has the new version section
head -n 20 CHANGELOG.md

# 3. Verify release notes file exists
ls "docs/release_notes/${RELEASE_VERSION%.*}.x/${RELEASE_VERSION}.md"

# 4. Verify Python package version matches VERSION (make release should have synced these)
grep -n '^version' protocol/python/pyproject.toml
grep -n '__version__' protocol/python/g8e/__init__.py
# Both should show X.Y.Z matching RELEASE_NUM

# 5. Find any doc version header (plain, bold, or "Document Version") NOT on the new
#    version — should return nothing for docs modified in this release. Docs NOT modified
#    are expected to still show the old version (that's the point). To check only modified
#    docs, pipe the git diff list:
#      git diff --name-only <prev-tag>..HEAD -- docs/ protocol/docs/ | xargs grep -niE '^(\*\*)?(document )?version:'
#    and verify each shows the new version. Unmodified docs are expected to lag.
grep -rniE '^(\*\*)?(document )?version:' docs/ protocol/docs/ --include='*.md' --exclude-dir=release_notes \
  | grep -viE "v?${RELEASE_NUM}([^0-9]|$)"

# 6. Find any "Last Updated" header not on the release date — should return nothing,
#    or only intentional entries (e.g., docs/devs/ docs on their own cadence, or docs
#    not modified in this release which are expected to keep their old date).
grep -rniE '^(\*\*)?last updated:' docs/ protocol/docs/ --include='*.md' --exclude-dir=release_notes \
  | grep -v "$RELEASE_DATE"
```

If step 4 shows a mismatch, run `make release` again to re-sync. Steps 5 and 6 will show old versions/dates for docs not modified in this release — that is expected and correct. Only investigate results for docs that *were* modified in this release (per the git diff).

---

## Release Workflow

The protocol (Go + Python) and platform binary share a single version number. There are no separate protocol releases. The `make release` / `make release-tag` workflow handles everything:

### `make release` — Prep (no git operations)

1. Syncs `protocol/python/pyproject.toml` and `protocol/python/g8e/__init__.py` from `VERSION`
2. Runs protocol tests
3. Builds binaries
4. Prints next-step instructions

### `make release-tag` — Tag & Push (run after committing)

1. Verifies working tree is clean
2. Verifies Python versions match `VERSION`
3. Verifies tags don't already exist
4. Creates `vX.Y.Z` and `protocol/vX.Y.Z` tags on the current commit
5. Pushes both tags to origin

### CI Workflows Triggered by Tags

| Tag | Workflow | What It Does |
|-----|----------|-------------|
| `vX.Y.Z` | `.github/workflows/release-binary.yml` | Builds all platforms, creates GitHub release with binary assets |
| `protocol/vX.Y.Z` | `.github/workflows/release-go-protocol.yml` | Creates GitHub release for Go module |
| `protocol/vX.Y.Z` | `.github/workflows/release-python-protocol.yml` | Builds and publishes Python package to PyPI |

Additionally, `.github/workflows/build-and-test.yml` includes a version sync check that fails CI if `pyproject.toml` or `__init__.py` don't match `VERSION`.

---

## Emergency Releases (Hotfixes)

For critical security issues or production bugs:

1. Apply the minimal fix necessary to the appropriate branch
2. Set `VERSION` and run `make release`
3. Update `CHANGELOG.md` and release notes
4. Complete the [Content Accuracy Review](#content-accuracy-review) for the touched areas
5. Commit and run `make release-tag`

---

## Out of Scope: Manual Git Steps

The only git operations not automated by `make release` / `make release-tag` are:

- Staging and committing the release-prep changes (`git add`, `git commit`)
- Pushing the branch before tagging (`git push`)

`make release-tag` handles tag creation and tag pushing automatically.

---

## References

- [Semantic Versioning](https://semver.org/spec/v2.0.0.html)
- [Keep a Changelog](https://keepachangelog.com/en/1.0.0/)
- [GitHub Releases Documentation](https://docs.github.com/en/repositories/releasing-projects-on-github/managing-releases-in-a-repository)
- [Go Module Versioning](https://go.dev/doc/modules/versioning)
- [PyPI Packaging](https://packaging.python.org/en/latest/tutorials/packaging-projects/)
