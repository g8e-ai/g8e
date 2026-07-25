# g8e Release Process

The primary purpose of a release is to **inventory every change since the last release and ensure the documentation accurately reflects the current state of the code.** Version bumps and CHANGELOG entries are mechanical byproducts of this work — the real value is in the change inventory and documentation reconciliation.

The protocol (Go + Python) and the platform binary share the same version number — there are no separate protocol releases.

> **`make release` handles version syncing, tagging, and pushing.** It does NOT build binaries, run lint or tests, or create GitHub releases — CI and GitHub Actions workflows handle those. The release prep changes (VERSION, CHANGELOG, release notes, doc updates) are committed and opened as a PR. After the PR is merged, pull main and run `make release` to tag and push. GitHub Actions workflows create the GitHub release and upload assets. See [Release Workflow](#release-workflow).

## How to Use This Document

1. **Inventory the changes** — Diff the release range and categorize every change (see [Change Inventory](#change-inventory)). This is the most important step — everything else depends on it.
2. **Update documentation to match the code** — Review every doc area against the change inventory. Fix inaccuracies, document missing features, remove stale references (see [Documentation Reconciliation](#documentation-reconciliation)). This is where the real work is.
3. **Write release notes** — Create `docs/release_notes/vX.Y.x/vX.Y.Z.md` from the change inventory (see [Release Notes](#release-notes)). The CHANGELOG entry is a summary of this.
4. **Bump version files** — Set `VERSION`, sync Python files, add CHANGELOG row (see [Version-Bearing Files](#version-bearing-files)). This is mechanical.
5. **Update doc headers** — Bump `Version:` / `Last Updated:` headers **only** in docs you actually modified in step 2 (see [Documentation Headers](#documentation-headers)).
6. **Run [Verification](#verification)** to catch any missed files.
7. **Commit and open a PR** — CI runs lint, tests, and version sync checks. Review, approve, and merge the PR on GitHub.
8. **After merge, pull main and run `make release`** to tag and push — GitHub Actions workflows create the release and upload assets (see [Release Workflow](#release-workflow)).

---

## Change Inventory

**This is the first and most important step.** You cannot write accurate release notes or update documentation without a complete inventory of what changed.

### Determine the Release Range

Find the previous release tag and diff from there to HEAD:

```bash
# Find the previous release tag
git tag -l 'v*' --sort=-version:refname | head -5

# If the tag exists locally:
git log --oneline v1.6.2..HEAD

# If the tag isn't fetched yet, use the commit SHA from the CHANGELOG or git log:
git log --oneline <prev-release-commit>..HEAD
```

### Categorize Every Change

For each commit in the release range, review the diff and categorize it:

```bash
# List all files changed
git diff --name-only <prev-tag>..HEAD

# Review each commit's diff in detail
git log --format='%H %s%n%b---' <prev-tag>..HEAD

# Get diff stats for a quick overview
git diff --stat <prev-tag>..HEAD
```

Categorize changes into:

- **Added** — New files, new functions, new features, new config options, new endpoints
- **Changed** — Refactored code, renamed files, changed signatures, changed behavior
- **Removed** — Deleted files, removed functions, removed endpoints, removed config
- **Fixed** — Bug fixes, corrected behavior
- **Security** — Security-related changes
- **Breaking** — Anything that requires user action or breaks compatibility

For each change, note:

1. **What changed** (file path, function/type name, old → new)
2. **Whether it's user-visible** (API surface, CLI flags, config keys, behavior) or internal-only
3. **Which docs reference the changed thing** — grep the docs for identifiers that were renamed, removed, or changed

### Find Stale Doc References

For every renamed, removed, or changed identifier, search the docs:

```bash
# Search docs for identifiers that were removed or renamed
grep -rnE 'OldName|old_command|OLD_CONSTANT|removedEndpoint' docs/ protocol/docs/ --include='*.md'

# Check which docs were modified in the release range
git diff --name-only <prev-tag>..HEAD -- docs/ protocol/docs/
```

Any doc that references something that was removed or renamed is stale and must be updated. Any doc that should document a new feature but doesn't is missing and must be added.

---

## Documentation Reconciliation

This is the core work of a release. The change inventory from the previous section tells you what changed — now make the docs match.

### When to Update a Doc

**Update a doc when one of these conditions is true:**

1. **Inaccuracy** — The doc describes something that is no longer true (a renamed command, a removed endpoint, a changed default, a corrected behavior). Fix the inaccurate prose so it matches the code.
2. **Missing feature** — The release adds a user-visible feature, command, endpoint, or config option that has no documentation at all. Add the missing documentation.
3. **Stale reference** — The doc references something that was removed or renamed in this release (e.g., a deprecated alias, a deleted route, a renamed constant, a deleted file). Remove or update the reference.

**Do NOT update a doc when:**

- The doc is already accurate — even if the underlying code was refactored internally, if the user-facing behavior and interface are unchanged, the doc is fine as-is.
- The doc wasn't touched by any code change in this release — leave its version header and content alone.
- You're tempted to "improve" prose that isn't wrong — cosmetic rewrites are not part of the release process.

> **Principle: Fix what's broken, document what's missing, leave the rest alone.** The goal is accuracy, not thoroughness. A doc that correctly describes the current behavior is done — don't rewrite it just because you read it.

### What to Review

Review the following documentation areas against the change inventory. For each area, only make edits if you find an inaccuracy or a missing feature per the rules above.

- **Protocol specs** (`protocol/docs/spec.md`, `a2a.md`, `mcp.md`) — If the protocol surface changed (endpoints, JSON-RPC methods, message shapes, auth flows), update the spec prose, not just the date. The Python (`protocol/python/`) and Go (`protocol/proto/`, generated code) bindings must agree with what the spec documents.
- **Architecture docs** (`docs/architecture/`) — If components, data flows, or security boundaries were refactored, reconcile the prose and diagrams. Pay special attention to: controller/service names, struct names, dependency wiring, and security pipeline descriptions.
- **Guides** (`docs/guides/`) — If CLI commands, flags, env vars, or setup steps changed, update the affected guides and any embedded command examples.
- **Glossary / compliance** (`docs/reference/`) — If terminology or control mappings changed, reconcile them.
- **Developer docs** (`docs/devs/`) — If code structure changed (new files, renamed files, deleted files, new packages), update `docs/devs/codemap.md` and any other relevant dev docs.
- **CHANGELOG / release notes** — Ensure every user-visible change (added, changed, removed, deprecated, fixed, security) is captured.

### How to Reconcile

For each stale reference or missing feature found in the change inventory:

1. Open the doc file
2. Find the stale or missing section
3. Update the prose to match the current code — use exact names, paths, and signatures from the actual source files
4. Update the `Version:` and `Last Updated:` headers on that file (see [Documentation Headers](#documentation-headers))
5. Record the doc as modified — you'll need this list for verification

---

## Release Notes

Create a new release notes file for every release. This is where the change inventory becomes a permanent record.

| # | File | Action |
|---|------|--------|
| 1 | `docs/release_notes/vX.Y.x/vX.Y.Z.md` | Create new file in the minor-version subdirectory (e.g., `v1.3.x/v1.3.1.md`) |

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
- **Optional sections**: `Deferred`, `Tests`, `Documentation`, `Dependencies`, and `Migration Notes for External Consumers` are not part of the Keep-a-Changelog standard but have been used in past release notes when those categories carry significant content. Use them when appropriate.
- **Detail level**: Release notes can be more verbose than the CHANGELOG entry — include file paths, function names, and links to relevant docs where helpful.
- **Trailing separator**: Some release notes end with a `---` horizontal rule. This is optional.

---

## Version-Bearing Files

After the change inventory and documentation reconciliation are complete, bump the version files. `VERSION` is the single source of truth — `make release` auto-syncs all derived files from it.

### Core Version Files

| # | File | How It's Updated | Format |
|---|------|-----------------|--------|
| 1 | `VERSION` | **Manual** — set to new version | `vX.Y.Z\n` (with trailing newline, no trailing spaces) |
| 2 | `CHANGELOG.md` | **Manual** — add a table row to the major-version section | `\| X.Y.Z \| YYYY-MM-DD \| ... \|` (no `v` prefix) |
| 3 | `protocol/python/pyproject.toml` | **Auto** — `make release` syncs from `VERSION` | `version = "X.Y.Z"` (no `v` prefix) |
| 4 | `protocol/python/g8e/__init__.py` | **Auto** — `make release` syncs from `VERSION` | `__version__ = "X.Y.Z"` (no `v` prefix) |

> ⚠️ Items 3 and 4 are auto-synced by `make release` and verified by CI. A mismatch will fail CI and the release.

#### CHANGELOG.md Format

The CHANGELOG is a table-based index. Each major version has a section (`## vX.Y.x`) containing a table of releases. Detailed content lives in the per-release notes files, not in the CHANGELOG itself.

If a major-version section already exists, add a new row at the top of the table. If this is a new major version, add a new section after the `---` separator.

```markdown
## vX.Y.x

| Version | Date | Description | Notes |
|---------|------|-------------|-------|
| X.Y.Z | YYYY-MM-DD | Brief summary of the release (1-3 sentences). | [vX.Y.Z](docs/release_notes/vX.Y.x/vX.Y.Z.md) |
```

The Description column should concisely summarize the release. The Notes column links to the full release notes file. Use the `v` prefix in the link text but not in the Version column.

### Versioning Rules

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

### Go Protocol Module

The Go protocol code is part of the root module `github.com/g8e-ai/g8e`. There is no separate `protocol/go.mod` to update. The Go module version is derived from git tags (`vX.Y.Z`), created by `make release`. External consumers use `go get github.com/g8e-ai/g8e@vX.Y.Z`.

### Build & CI Files (No Version Update Required)

The following files read the version dynamically from `VERSION` at build time and do **not** require manual updates:

- `Makefile` — Reads `VERSION` via `$(shell cat VERSION)`; `make release` syncs Python files, tags, and pushes (GitHub Actions workflows create the release)
- `Dockerfile` — Receives version as build arg
- `docker-compose.yml` — References build context, not version
- `.github/workflows/*.yml` — Triggered by git tags, no hardcoded version. CI includes a version sync check that fails if Python files don't match `VERSION`.

---

## Documentation Headers

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

#### Architecture Docs (`docs/architecture/`) — 10 files

| # | File |
|---|------|
| 1 | `docs/architecture/auth.md` |
| 2 | `docs/architecture/encryption.md` |
| 3 | `docs/architecture/gateway.md` |
| 4 | `docs/architecture/governance.md` |
| 5 | `docs/architecture/network.md` |
| 6 | `docs/architecture/operator.md` |
| 7 | `docs/architecture/protocol.md` |
| 8 | `docs/architecture/scripts.md` |
| 9 | `docs/architecture/sse.md` |
| 10 | `docs/architecture/storage.md` |

#### Guide Docs (`docs/guides/`) — 11 files

| # | File | Note |
|---|------|------|
| 11 | `docs/guides/air_gap.md` | |
| 12 | `docs/guides/build_apps.md` | |
| 13 | `docs/guides/build_gateway.md` | |
| 14 | `docs/guides/build_operator.md` | |
| 15 | `docs/guides/cloudflare_tunnel.md` | |
| 16 | `docs/guides/connect_apps_to_gateway.md` | |
| 17 | `docs/guides/connect_operator_to_gateway.md` | |
| 18 | `docs/guides/docker_gateway.md` | |
| 19 | `docs/guides/getting_started.md` | |
| 20 | `docs/guides/gui_enrollment.md` | |
| 21 | `docs/guides/lovable.md` | |


#### Reference Docs (`docs/reference/`) — 2 files

| # | File | Note |
|---|------|------|
| 22 | `docs/reference/glossary.md` | plain `Version:` header |
| 23 | `docs/reference/compliance-alignment.md` | `**Document Version:**` (no `v` prefix) |

#### Protocol Docs (`protocol/docs/`) — 3 files

| # | File | Note |
|---|------|------|
| 24 | `protocol/docs/spec.md` | has both `Version:` and `Last Updated:` |
| 25 | `protocol/docs/a2a.md` | has both `Version:` and `Last Updated:` |
| 26 | `protocol/docs/mcp.md` | has both `Version:` and `Last Updated:` |

#### Update Pattern for Each Modified Doc

For plain-format files, update the two header lines:

```diff
-Last Updated: 2026-06-24
-Version: v1.3.0
+Last Updated: YYYY-MM-DD
+Version: vX.Y.Z
```

For bold-format and document-version files, update whichever of `Version`, `Document Version`, and `Last Updated` lines are present, preserving the existing markdown styling.

> **Only update docs you actually reviewed.** If a doc's content didn't change in this release, leave its headers at the previous version. This keeps version stamps meaningful — they reflect the last time a human reviewed and updated the doc, not the last release tag.

### Docs Without Version Headers (No Version Update Required)

The following doc directories intentionally do **not** carry release `Version:` headers and do **not** need version bumps on release. They may still need *content* updates if the release changes their subject matter (see [Documentation Reconciliation](#documentation-reconciliation)):

- `docs/core/` — Position papers, about page
- `docs/devs/` — Developer documentation (including this file). Note: some devs docs (e.g., `docs/devs/tests.md`, `docs/devs/troubleshooting.md`) carry `Last Updated:` dates and/or `Version:` headers that track the doc's own content changes, **not** the release — only bump them if you changed the doc.
- `docs/diagrams/` — Mermaid diagrams and flowcharts
- `docs/release_notes/` — Historical release notes (past entries are immutable)
- `demos/` — Demo configurations and doctrine files
- `README.md` — Uses a dynamic GitHub badge for latest release; no hardcoded version

---

## Manual Updates Checklist

`make release` handles version syncing, tagging, and pushing. Lint and tests are handled by CI on PRs. GitHub Actions workflows handle release creation and asset uploads. The following must still be done manually:

- [ ] **1. Inventory changes** — Diff the release range and categorize every change (see [Change Inventory](#change-inventory))
- [ ] **2. Reconcile documentation** — Review all doc areas against the change inventory. Fix inaccuracies, document missing features, remove stale references (see [Documentation Reconciliation](#documentation-reconciliation))
- [ ] **3. Write release notes** — Create `docs/release_notes/vX.Y.x/vX.Y.Z.md` from the change inventory
- [ ] **4. `VERSION`** — Set to `vX.Y.Z`
- [ ] **5. Sync Python files** — Run `make release` to auto-sync `pyproject.toml` + `__init__.py` from `VERSION` (it will sync and exit due to dirty working tree), or update both files manually
- [ ] **6. `CHANGELOG.md`** — Add a table row to the major-version section (no `v` prefix in version column)
- [ ] **7. Documentation headers** — Update version/date headers **only** in docs that were actually modified in step 2 (use `git diff --name-only <prev-tag>..HEAD -- docs/ protocol/docs/` to identify them). Do not blanket-bump all headers.
- [ ] **8. Run [Verification](#verification)** to catch any missed files
- [ ] **9. Commit and open PR** — `git add -A && git commit -m "release: vX.Y.Z"`, push, and open a PR on GitHub. CI runs lint, tests, and version sync checks.
- [ ] **10. Merge and release** — After the PR is merged, pull main and run `make release` to tag and push — GitHub Actions workflows create the release and upload assets

**Only 2 files need manual version edits** (VERSION + CHANGELOG). The Python package files are auto-synced. Everything else is content-driven work — inventory, docs, and release notes.

> **Workflow note:** All release prep (steps 1–9) happens on a feature branch and is merged via PR. Tagging and pushing (step 10) happens on the merged main branch via `make release`; GitHub Actions workflows handle release creation and asset uploads.

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

# 7. Verify no stale references remain — grep for identifiers that were removed or
#    renamed in this release and confirm no docs still reference them.
grep -rnE 'OldName|old_command|OLD_CONSTANT' docs/ protocol/docs/ --include='*.md'
```

If step 4 shows a mismatch, run `make release` again to re-sync. Steps 5 and 6 will show old versions/dates for docs not modified in this release — that is expected and correct. Only investigate results for docs that *were* modified in this release (per the git diff). Step 7 should return nothing — if it finds stale references, fix them before committing.

---

## Release Workflow

The protocol (Go + Python) and platform binary share a single version number. There are no separate protocol releases. The `make release` target handles version syncing, tagging, and pushing in a single step. CI handles lint, tests, and version sync verification on PRs. GitHub Actions workflows handle release creation and asset uploads.

### `make release` — Tag and Push

> **Run this on the merged main branch**, not on a feature branch. The tags must point at the merge commit on main.

1. Syncs `protocol/python/pyproject.toml` and `protocol/python/g8e/__init__.py` from `VERSION` (if already in sync, no changes are made)
2. Verifies working tree is clean (fails if Python files were out of sync; commit synced files and go through the PR process first)
3. Verifies release notes file exists at `docs/release_notes/vX.Y.x/vX.Y.Z.md`
4. Verifies tags `vX.Y.Z` and `protocol/vX.Y.Z` don't already exist
5. Creates `vX.Y.Z` and `protocol/vX.Y.Z` tags on the current commit
6. Pushes both tags to origin

The `vX.Y.Z` tag triggers the `release-binary.yml` workflow, which builds all platforms, signs binaries, creates the GitHub release, and uploads assets. The `protocol/vX.Y.Z` tag triggers the `release-python-protocol.yml` workflow, which publishes the Python package to PyPI.

> **Lint and tests are handled by CI** (`.github/workflows/build-and-test.yml`) on pull requests, not by `make release`. The CI workflow includes a version sync check that fails if `pyproject.toml` or `__init__.py` don't match `VERSION`.

The `protocol/v*` tag triggers the Python PyPI release workflow only. The Go module is versioned by the `v*` tag.

### CI Workflows Triggered by Tags

| Tag | Workflow | What It Does |
|-----|----------|-------------|
| `vX.Y.Z` | `.github/workflows/release-binary.yml` | Builds all platforms, signs binaries with cosign, uploads assets to GitHub release, and verifies fresh `go install` works on Ubuntu, macOS, and Windows |
| `protocol/vX.Y.Z` | `.github/workflows/release-python-protocol.yml` | Builds and publishes Python package to PyPI, verifies fresh PyPI install and imports on Ubuntu, macOS, and Windows |

The `protocol/v*` tag is used only as a trigger for the Python PyPI release workflow. It is NOT used for Go module versioning — the Go module is part of the root module and is versioned by `v*` tags.

---

## Emergency Releases (Hotfixes)

For critical security issues or production bugs:

1. Apply the minimal fix necessary to the appropriate branch
2. Inventory the changes and reconcile documentation for the touched areas
3. Set `VERSION`, sync Python files, update `CHANGELOG.md` and release notes
4. Commit, open a PR, merge, then pull main and run `make release` to tag and push — GitHub Actions workflows create the release

---

## Out of Scope: Manual Git Steps

The only git operations not automated by `make release` are:

- Staging and committing the release-prep changes (`git add`, `git commit`)
- Pushing the branch and opening a PR (`git push`, GitHub PR)
- Merging the PR on GitHub
- Pulling main after merge (`git checkout main && git pull`)

`make release` handles tag creation and tag pushing automatically — run it on the merged main branch. GitHub Actions workflows handle release creation and asset uploads.

---

## References

- [Semantic Versioning](https://semver.org/spec/v2.0.0.html)
- [Keep a Changelog](https://keepachangelog.com/en/1.0.0/)
- [GitHub Releases Documentation](https://docs.github.com/en/repositories/releasing-projects-on-github/managing-releases-in-a-repository)
- [Go Module Versioning](https://go.dev/doc/modules/versioning)
- [PyPI Packaging](https://packaging.python.org/en/latest/tutorials/packaging-projects/)
