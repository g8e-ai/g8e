# g8e Release Process — Version & Documentation Updates

This document is the definitive checklist for updating all version-bearing files and documentation when bumping a g8e release. Every file listed here must be updated and verified before the release is handed off for tagging.

> **Git is maintainer-only.** This document covers *content and version updates only*. No permanent git operations (`git add`, `git commit`, `git tag`, `git push`) are part of this process — those are performed manually by the release owner. See [Out of Scope: Maintainer Git Steps](#out-of-scope-maintainer-git-steps). Read-only inspection (`git status`, `git diff`) is fine for sanity-checking, but is not required by this checklist.

## How to Use This Document

1. Determine the new version number (see [Versioning Rules](#versioning-rules))
2. Work through the [Version-Bearing Files](#version-bearing-files) section top-to-bottom
3. Review release content for [documentation accuracy](#content-accuracy-review) — protocol specs, architecture docs, and guides must reflect any code refactors or behavioral changes shipped in this release
4. Run the [Verification](#verification) section to catch any missed files
5. Hand off to the maintainer for git tagging (out of scope here)

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

The following files contain version strings that must be updated on every release. They are grouped by category.

### A. Core Version Files

These are the canonical files that define the platform and protocol versions.

| # | File | What to Update | Format |
|---|------|---------------|--------|
| 1 | `VERSION` | Entire file contents | `vX.Y.Z\n` (with trailing newline, no trailing spaces) |
| 2 | `CHANGELOG.md` | Add new section below the `## [Unreleased]` block | `## [X.Y.Z] - YYYY-MM-DD` (no `v` prefix) |
| 3 | `protocol/python/pyproject.toml` | `version` field in `[project]` section | `version = "X.Y.Z"` (no `v` prefix) |
| 4 | `protocol/python/g8e/__init__.py` | `__version__` constant | `__version__ = "X.Y.Z"` (no `v` prefix) — **must match `pyproject.toml`** |

> ⚠️ Items 3 and 4 are the Python package version and **must always be identical**. A mismatch will produce an inconsistent published package.

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
> | Bold | `**Version:** vX.Y.Z` / `**Last Updated:** YYYY-MM-DD` | `docs/guides/build_agentic_system.md` |
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

#### Guide Docs (`docs/guides/`) — 10 files

| # | File | Note |
|---|------|------|
| 14 | `docs/guides/air_gap.md` | |
| 15 | `docs/guides/build_agentic_system.md` | **bold** header format |
| 16 | `docs/guides/build_apps.md` | |
| 17 | `docs/guides/build_gateway.md` | |
| 18 | `docs/guides/build_operator.md` | |
| 19 | `docs/guides/cli.md` | |
| 20 | `docs/guides/connect_apps_to_gateway.md` | |
| 21 | `docs/guides/connect_operator_to_gateway.md` | |
| 22 | `docs/guides/docker_gateway.md` | |
| 23 | `docs/guides/getting_started.md` | |

#### Reference Docs (`docs/reference/`) — 2 files

| # | File | Note |
|---|------|------|
| 24 | `docs/reference/glossary.md` | plain `Version:` header |
| 25 | `docs/reference/compliance-alignment.md` | `**Document Version:**` (no `v` prefix) |

#### Protocol Docs (`protocol/docs/`) — 3 files

| # | File | Note |
|---|------|------|
| 26 | `protocol/docs/spec.md` | has both `Version:` and `Last Updated:` |
| 27 | `protocol/docs/a2a.md` | `Last Updated:` only — update the date |
| 28 | `protocol/docs/mcp.md` | `Last Updated:` only — update the date |

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
| `protocol/go.mod` | No version field to update. The Go module version is derived from git tags (`protocol/vX.Y.Z`), which the maintainer creates. |

### F. Build & CI Files (No Version Update Required)

The following files read the version dynamically from `VERSION` at build time and do **not** require manual updates:

- `Makefile` — Reads `VERSION` via `$(shell cat VERSION)`
- `Dockerfile` / `Dockerfile.operator` — Receive version as build arg
- `docker-compose.yml` — References build context, not version
- `.github/workflows/*.yml` — Triggered by git tags, no hardcoded version

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

## Complete Update Checklist

When preparing a release, work through this checklist in order:

- [ ] **1. `VERSION`** — Set to `vX.Y.Z`
- [ ] **2. `CHANGELOG.md`** — Add `## [X.Y.Z] - YYYY-MM-DD` section (no `v` prefix)
- [ ] **3. `protocol/python/pyproject.toml`** — Set `version = "X.Y.Z"` (no `v` prefix)
- [ ] **4. `protocol/python/g8e/__init__.py`** — Set `__version__ = "X.Y.Z"` (must match item 3)
- [ ] **5. `docs/release_notes/vX.Y.x/vX.Y.Z.md`** — Create new release notes file
- [ ] **6–28. Documentation headers** — Update version/date headers **only** in docs that were actually reviewed or modified in this release (use `git diff --name-only <prev-tag>..HEAD -- docs/ protocol/docs/` to identify them). Do not blanket-bump all headers. See [Section C](#c-documentation-with-version--date-headers) for the full list of files that carry headers.
- [ ] **Content review** — Complete the [Content Accuracy Review](#content-accuracy-review). Only update docs to fix inaccuracies or document missing features — do not rewrite accurate docs.

**Total: 5 core files to update on every release** (VERSION, CHANGELOG, Python package ×2, release notes), plus version/date header updates **only in docs actually modified** in the release, plus a targeted content review fixing inaccuracies and documenting missing features in docs affected by code changes.

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

# 4. Verify Python package version matches in BOTH files (must be identical)
grep -n '^version' protocol/python/pyproject.toml
grep -n '__version__' protocol/python/g8e/__init__.py

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

If step 4 shows a mismatch between the two Python files, reconcile them. Steps 5 and 6 will show old versions/dates for docs not modified in this release — that is expected and correct. Only investigate results for docs that *were* modified in this release (per the git diff).

---

## Protocol Release Notes (If Applicable)

Protocol packages (Go and Python) may be released independently of platform releases. If protocol changes are included:

1. The Go and Python packages must use the same version number
2. The Python version is set in `protocol/python/pyproject.toml` **and** `protocol/python/g8e/__init__.py` (covered by the checklist above — keep them identical)
3. The Go module version derives from the git tag (`protocol/vX.Y.Z`), created by the maintainer
4. The `protocol/vX.Y.Z` tag triggers both release workflows:
   - **Go**: `.github/workflows/release-go-protocol.yml`
   - **Python**: `.github/workflows/release-python-protocol.yml`

This is informational — tag creation is a [maintainer git step](#out-of-scope-maintainer-git-steps).

---

## Emergency Releases (Hotfixes)

For critical security issues or production bugs:

1. Apply the minimal fix necessary to the appropriate branch
2. Work through the [Complete Update Checklist](#complete-update-checklist) above
3. Complete the [Content Accuracy Review](#content-accuracy-review) for the touched areas
4. Hand off to the maintainer for tagging and release

---

## Out of Scope: Maintainer Git Steps

The following are performed **manually by the release owner** and are deliberately **not** part of this checklist. Do not run them as part of preparing the release content:

- Staging and committing the release-prep changes
- Creating the release tag (`vX.Y.Z`) and any protocol tag (`protocol/vX.Y.Z`)
- Pushing branches and tags
- Publishing the GitHub release

The job of this document is to leave the working tree fully and correctly updated so the maintainer can perform those git steps with confidence.

---

## References

- [Semantic Versioning](https://semver.org/spec/v2.0.0.html)
- [Keep a Changelog](https://keepachangelog.com/en/1.0.0/)
- [GitHub Releases Documentation](https://docs.github.com/en/repositories/releasing-projects-on-github/managing-releases-in-a-repository)
- [Go Module Versioning](https://go.dev/doc/modules/versioning)
- [PyPI Packaging](https://packaging.python.org/en/latest/tutorials/packaging-projects/)
