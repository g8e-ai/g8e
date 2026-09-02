# g8e Release Process

The primary purpose of a release is to **inventory every change since the last release and ensure the documentation accurately reflects the current state of the code.** Version bumps and CHANGELOG entries are mechanical byproducts of this work; the real value is in the change inventory and documentation reconciliation.

The protocol (Go + Python) and the platform binary share the same version number. There are no separate protocol releases.

> **`make release` handles version syncing, tagging, and pushing.** It does NOT build binaries, run lint or tests, or create GitHub releases; CI and GitHub Actions workflows handle those. Release prep changes are committed and opened as a PR; after merge, pull main and run `make release` to tag and push. See [Release Workflow](#release-workflow).

## Separation of Duties

Release work is split between the **agent** (PR prep) and the **release owner** (merge, CI gate, tag/push). The agent never runs `make release`, never commits, and never pushes.

**Agent (PR prep, on a feature branch):**
1. Inventory the changes since the previous release tag
2. Reconcile documentation with the code changes
3. Write `docs/release_notes/vX.Y.x/vX.Y.Z.md`
4. Set `VERSION` to `vX.Y.Z`, add the `CHANGELOG.md` row, and sync the Python package files (`protocol/python/pyproject.toml`, `protocol/python/g8e/__init__.py`, and the editable `g8e` package entry in `protocol/python/uv.lock`) to `X.Y.Z` (no `v` prefix) so CI's version sync and locked-environment checks pass on the PR
5. Run the read-only [Verification](#verification) checks (all steps should pass, including step 4)
6. Stop. The agent does NOT commit, push, open the PR, or run `make release`. Hand the prepared working tree back to the release owner.

**Release owner (commits, merges, tags, pushes):**
1. Review the prepared changes, then `git add`, `git commit`, `git push`, and open the PR on GitHub
2. Merge the PR on GitHub
3. Wait for CI on `main` to pass (lint, tests, version sync checks)
4. `git checkout main && git pull` locally
5. Run `make release` — this re-syncs the Python package files from `VERSION` (a no-op if the agent already synced them), then creates and pushes the `vX.Y.Z` and `protocol/vX.Y.Z` tags
6. GitHub Actions workflows create the GitHub release, build and sign binaries, and publish the Python package to PyPI

The Python package files (`pyproject.toml`, `__init__.py`, and the editable package entry in `uv.lock`) are synced to the new version during PR prep by the agent (manually, since the agent cannot run `make release`) so CI's version sync and locked-environment checks pass on the PR. `make release` re-syncs them after merge as a no-op safety net. The agent never runs `make release`, never commits, and never pushes.

## How to Use This Document

1. **Inventory the changes**: Diff the release range and categorize every change (see [Change Inventory](#change-inventory)). This is the most important step; everything else depends on it.
2. **Update documentation to match the code**: For every doc in scope, do a full review of the entire doc against the current code. Fix inaccuracies, document missing features, remove stale references, and bump the `Version:`/`Last Updated:` headers in the same pass (see [Documentation Reconciliation](#documentation-reconciliation)). This is where the real work is.
3. **Write release notes**: Create `docs/release_notes/vX.Y.x/vX.Y.Z.md` from the change inventory (see [Release Notes](#release-notes)). The CHANGELOG entry is a summary of this.
4. **Bump version files**: Set `VERSION`, sync the Python package files (`pyproject.toml`, `__init__.py`, and `uv.lock`) to match, and add the CHANGELOG row (see [Version-Bearing Files](#version-bearing-files)). This is mechanical. The Python files must be synced during PR prep so CI's version sync check passes on the PR — `make release` re-syncs them after merge as a no-op safety net.
5. **Run [Verification](#verification)** to catch any missed files — including docs modified in step 2 that didn't get their headers bumped.
6. **Hand the prepared working tree back to the release owner.** The agent stops here — it does NOT commit, push, open a PR, or run `make release`. See [Separation of Duties](#separation-of-duties).
7. **Release owner**: commit, push, open and merge the PR; after CI on `main` passes, pull main and run `make release` to tag and push; GitHub Actions workflows create the release and upload assets (see [Release Workflow](#release-workflow)).

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

- **Added**: New files, new functions, new features, new config options, new endpoints
- **Changed**: Refactored code, renamed files, changed signatures, changed behavior
- **Removed**: Deleted files, removed functions, removed endpoints, removed config
- **Fixed**: Bug fixes, corrected behavior
- **Security**: Security-related changes
- **Breaking**: Anything that requires user action or breaks compatibility

For each change, note:

1. **What changed** (file path, function/type name, old → new)
2. **Whether it's user-visible** (API surface, CLI flags, config keys, behavior) or internal-only
3. **Which docs reference the changed thing**: grep the docs for identifiers that were renamed, removed, or changed

### Find Stale Doc References

For every renamed, removed, or changed identifier, search the docs:

```bash
# Search docs for identifiers that were removed or renamed
grep -rnE 'OldName|old_command|OLD_CONSTANT|removedEndpoint' docs/ protocol/docs/ --include='*.md'

# Find docs with explicit g8e version callouts (go get, pip install, pip download) that
# reference the previous release version. These must be updated every release.
grep -rnE "g8e==[0-9]+\.[0-9]+\.[0-9]+|g8e-ai/g8e/v2@v[0-9]+\.[0-9]+\.[0-9]+" docs/ protocol/docs/ README.md protocol/README.md --include='*.md' \
  | grep -v release_notes | grep -v CHANGELOG

# Check which docs were modified in the release range
git diff --name-only <prev-tag>..HEAD -- docs/ protocol/docs/
```

Any doc that references something that was removed or renamed is stale and must be updated. Any doc that should document a new feature but doesn't is missing and must be added. Any doc with a version callout referencing the previous release version must be updated to the new release version, regardless of whether it was touched by a code change.

---

## Documentation Reconciliation

This is the core work of a release. The change inventory from the previous section tells you what changed; now make the docs match.

### When to Update a Doc

**Update a doc when one of these conditions is true:**

1. **Inaccuracy**: The doc describes something that is no longer true (a renamed command, a removed endpoint, a changed default, a corrected behavior). Fix the inaccurate prose so it matches the code.
2. **Missing feature**: The release adds a user-visible feature, command, endpoint, or config option that has no documentation at all. Add the missing documentation.
3. **Stale reference**: The doc references something that was removed or renamed in this release (e.g., a deprecated alias, a deleted route, a renamed constant, a deleted file). Remove or update the reference.
4. **Stale version callout**: The doc contains an explicit version callout — a `go get ...@vX.Y.Z`, `pip install g8e==X.Y.Z`, `pip download g8e==X.Y.Z`, or any other install/fetch command that pins a specific release version — that references the previous release version. These callouts are version-bearing content and must be updated to the new release version in every release, regardless of whether the doc was touched by a code change. Bump the doc's `Version:`/`Last Updated:` headers in the same pass since the content changed.

**Do NOT update a doc when:**

- The doc is already accurate; even if the underlying code was refactored internally, if the user-facing behavior and interface are unchanged, the doc is fine as-is.
- The doc wasn't touched by any code change in this release and contains no stale version callouts; leave its version header and content alone.
- You're tempted to "improve" prose that isn't wrong; cosmetic rewrites are not part of the release process.

> **Principle: Fix what's broken, document what's missing, leave the rest alone.** The goal is accuracy, not thoroughness. A doc that correctly describes the current behavior is done; don't rewrite it just because you read it.

### What to Review

Review the following documentation areas against the change inventory. For each area, only make edits if you find an inaccuracy or a missing feature per the rules above.

- **Protocol specs** (`protocol/docs/spec.md`, `a2a.md`, `mcp.md`, `constants.md`): If the protocol surface changed (endpoints, JSON-RPC methods, message shapes, auth flows), update the spec prose, not just the date. The Python (`protocol/python/`) and Go (`protocol/proto/`, generated code) bindings must agree with what the spec documents.
- **Architecture docs** (`docs/architecture/`): If components, data flows, or security boundaries were refactored, reconcile the prose and diagrams. Pay special attention to: controller/service names, struct names, dependency wiring, and security pipeline descriptions.
- **Guides** (`docs/guides/`): If CLI commands, flags, env vars, or setup steps changed, update the affected guides and any embedded command examples.
- **Glossary / compliance** (`docs/reference/`): If terminology or control mappings changed, reconcile them.
- **Developer docs** (`docs/devs/`): If code structure changed (new files, renamed files, deleted files, new packages), update `docs/devs/codemap.md` and any other relevant dev docs.
- **CHANGELOG / release notes**: Ensure every user-visible change (added, changed, removed, deprecated, fixed, security) is captured.

### How to Reconcile

For each doc that falls in scope (any doc touched by a code change in the release, any doc that references an identifier that was renamed, removed, or changed, or any doc that contains a stale version callout per condition 4 above), do a **full review of the entire doc against the current code** — not just the specific stale reference that flagged it. Read the whole file, verify every claim, command, path, signature, and behavior description against the actual source, and update everything that is inaccurate or missing. The version header bump is part of this review, not a separate mechanical step: a doc whose content was reviewed and updated gets its `Version:` and `Last Updated:` headers bumped in the same pass.

1. Open the doc file
2. Read the entire doc end to end, checking every statement against the current code
3. Update all inaccurate or missing prose to match the current code; use exact names, paths, and signatures from the actual source files
4. Bump the `Version:` and `Last Updated:` headers on that file as part of the same edit (see [Documentation Headers](#documentation-headers)) — a reviewed-and-updated doc must carry the new version stamp
5. Record the doc as modified; you'll need this list for verification

A doc that was flagged as in scope but, on full review, turns out to already be accurate (nothing to fix) does **not** get a version bump and is not recorded as modified — the header reflects the last time a human changed the content, not the last time a human read it.

---

## Release Notes

Create a new release notes file for every release. This is where the change inventory becomes a permanent record.

| # | File | Action |
|---|------|--------|
| 1 | `docs/release_notes/vX.Y.x/vX.Y.Z.md` | Create new file in the minor-version subdirectory (e.g., `v1.3.x/v1.3.1.md`) |

The release notes file should mirror the CHANGELOG entry but can be more detailed. Include the version header and date, an Overview, all applicable Keep-a-Changelog sections, and any additional context, examples, or links to relevant documentation. Past release notes are immutable; never edit historical entries.

#### Release Notes Template

Use the same `## [X.Y.Z] - YYYY-MM-DD` header as the CHANGELOG (no `v` prefix in the bracket). Only include the subsections that apply to the release; most releases use 2-4 of these, not all of them.

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
- **Detail level**: Release notes can be more verbose than the CHANGELOG entry; include file paths, function names, and links to relevant docs where helpful.
- **Trailing separator**: Some release notes end with a `---` horizontal rule. This is optional.

---

## Version-Bearing Files

After the change inventory and documentation reconciliation are complete, bump the version files. `VERSION` is the single source of truth; `make release` auto-syncs all derived files from it.

### Core Version Files

| # | File | How It's Updated | Format |
|---|------|-----------------|--------|
| 1 | `VERSION` | **Manual** (agent, PR prep): set to new version | `vX.Y.Z\n` (with trailing newline, no trailing spaces) |
| 2 | `CHANGELOG.md` | **Manual** (agent, PR prep): add a table row to the major-version section | `\| X.Y.Z \| YYYY-MM-DD \| ... \|` (no `v` prefix) |
| 3 | `protocol/python/pyproject.toml` | **Manual** (agent, PR prep): set to match `VERSION` so CI passes; `make release` re-syncs after merge as a no-op safety net | `version = "X.Y.Z"` (no `v` prefix) |
| 4 | `protocol/python/g8e/__init__.py` | **Manual** (agent, PR prep): set to match `VERSION` so CI passes; `make release` re-syncs after merge as a no-op safety net | `__version__ = "X.Y.Z"` (no `v` prefix) |
| 5 | `protocol/python/uv.lock` | **Manual** (agent, PR prep): set the editable `g8e` package entry to match `VERSION` so locked environments remain usable; `make release` re-syncs after merge as a no-op safety net | `version = "X.Y.Z"` under `name = "g8e"` (no `v` prefix) |

> Items 3 through 5 must be synced to `VERSION` during PR prep so CI's version sync check passes on the PR. The agent edits them manually (it cannot run `make release`). `make release` re-syncs them after merge as a no-op safety net. A mismatch will fail CI.

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

The Go protocol code is part of the root module `github.com/g8e-ai/g8e/v2`. There is no separate `protocol/go.mod` to update. The Go module version is derived from git tags (`vX.Y.Z`), created by `make release`. External consumers use `go get github.com/g8e-ai/g8e/v2@vX.Y.Z`.

### Build & CI Files (No Version Update Required)

The following files read the version dynamically from `VERSION` at build time and do **not** require manual updates:

- `Makefile`: Reads `VERSION` via `$(shell cat VERSION)`; `make release` syncs Python files, tags, and pushes (GitHub Actions workflows create the release)
- `Dockerfile`: Builds via `make build-all`, which reads `VERSION` from the file at build time (no version build arg)
- `docker-compose.yml`: References build context, not version
- `.github/workflows/*.yml`: Triggered by git tags, no hardcoded version. CI includes a version sync check that fails if Python files don't match `VERSION`.

---

## Documentation Headers

Every markdown file below carries a version and/or `Last Updated` header near the top. **Only update headers in docs that were actually reviewed or modified as part of this release.** Do not blanket-bump all headers at release time; a doc whose content hasn't changed should not get a new version stamp.

To determine which docs need header updates, diff the release range:

```bash
git diff --name-only <previous-tag>..HEAD -- docs/ protocol/docs/
```

Any file in the output that carries a version/date header should have that header updated to the new version and release date. Files not in the output are left as-is.

> **Header formats vary**: they are not uniform across the repo. The [Verification](#verification) grep is written to catch all of them, but when editing by hand watch for:
>
> | Format | Example | Files |
> |--------|---------|-------|
> | Plain | `Version: vX.Y.Z` / `Last Updated: YYYY-MM-DD` | most architecture, guide, reference, and protocol docs |
> | Bold | `**Version:** vX.Y.Z` / `**Last Updated:** YYYY-MM-DD` | (none currently) |
> | Document Version (no `v`) | `**Document Version:** X.Y.Z` | `docs/reference/compliance-alignment.md` |

#### Architecture Docs (`docs/architecture/`): 15 files

| # | File |
|---|------|
| 1 | `docs/architecture/agents.md` |
| 2 | `docs/architecture/auth.md` |
| 3 | `docs/architecture/consensus.md` |
| 4 | `docs/architecture/dashboard.md` |
| 5 | `docs/architecture/encryption.md` |
| 6 | `docs/architecture/ensemble.md` |
| 7 | `docs/architecture/gateway.md` |
| 8 | `docs/architecture/governance.md` |
| 9 | `docs/architecture/network.md` |
| 10 | `docs/architecture/operator.md` |
| 11 | `docs/architecture/overview.md` |
| 12 | `docs/architecture/protocol.md` |
| 13 | `docs/architecture/scripts.md` |
| 14 | `docs/architecture/sse.md` |
| 15 | `docs/architecture/storage.md` |

#### Guide Docs (`docs/guides/`): 13 files

| # | File | Note |
|---|------|------|
| 16 | `docs/guides/air_gap.md` | |
| 17 | `docs/guides/build_apps.md` | |
| 18 | `docs/guides/build_frontend.md` | |
| 19 | `docs/guides/build_gateway.md` | |
| 20 | `docs/guides/build_operator.md` | |
| 21 | `docs/guides/cloudflare_tunnel.md` | |
| 22 | `docs/guides/connect_apps_to_gateway.md` | |
| 23 | `docs/guides/connect_frontend_to_gateway.md` | |
| 24 | `docs/guides/connect_operator_to_gateway.md` | |
| 25 | `docs/guides/docker_gateway.md` | |
| 26 | `docs/guides/getting_started.md` | |
| 27 | `docs/guides/lovable.md` | |
| 28 | `docs/guides/unified_stack.md` | |


#### Reference Docs (`docs/reference/`): 3 files

| # | File | Note |
|---|------|------|
| 29 | `docs/reference/glossary.md` | plain `Version:` header |
| 30 | `docs/reference/compliance-alignment.md` | `**Document Version:**` (no `v` prefix) |
| 31 | `docs/reference/fips140-3.md` | plain `Version:` header |

#### Protocol Docs (`protocol/docs/`): 3 files

| # | File | Note |
|---|------|------|
| 32 | `protocol/docs/spec.md` | has both `Version:` and `Last Updated:` |
| 33 | `protocol/docs/a2a.md` | has both `Version:` and `Last Updated:` |
| 34 | `protocol/docs/mcp.md` | has both `Version:` and `Last Updated:` |

#### Update Pattern for Each Modified Doc

For plain-format files, update the two header lines:

```diff
-Last Updated: 2026-06-24
-Version: v1.3.0
+Last Updated: YYYY-MM-DD
+Version: vX.Y.Z
```

For bold-format and document-version files, update whichever of `Version`, `Document Version`, and `Last Updated` lines are present, preserving the existing markdown styling.

> **Only update docs you actually reviewed.** If a doc's content didn't change in this release, leave its headers at the previous version. This keeps version stamps meaningful; they reflect the last time a human reviewed and updated the doc, not the last release tag.

### Docs Without Version Headers (No Version Update Required)

The following doc directories intentionally do **not** carry release `Version:` headers and do **not** need version bumps on release. They may still need *content* updates if the release changes their subject matter (see [Documentation Reconciliation](#documentation-reconciliation)):

- `docs/core/`: Position papers, about page
- `docs/devs/`: Developer documentation (including this file). Note: some devs docs (e.g., `docs/devs/tests.md`, `docs/devs/troubleshooting.md`) carry `Last Updated:` dates and/or `Version:` headers that track the doc's own content changes, **not** the release; only bump them if you changed the doc.
- `docs/diagrams/`: Mermaid diagrams and flowcharts
- `docs/release_notes/`: Historical release notes (past entries are immutable)
- `demos/`: Demo configurations and doctrine files
- `README.md`: Uses a dynamic GitHub badge for latest release; no hardcoded version

---

## Manual Updates Checklist

`make release` handles version syncing, tagging, and pushing (see [Release Workflow](#release-workflow)). The following must still be done manually:

- [ ] **1. Inventory changes**: Diff the release range and categorize every change (see [Change Inventory](#change-inventory))
- [ ] **2. Reconcile documentation**: For every doc in scope (touched by a code change, referencing a renamed/removed/changed identifier, or containing a stale version callout), do a full review of the entire doc against the current code. Fix inaccuracies, document missing features, remove stale references, update stale version callouts (`go get ...@vX.Y.Z`, `pip install g8e==X.Y.Z`, `pip download g8e==X.Y.Z`) to the new release version, and bump the `Version:`/`Last Updated:` headers in the same pass (see [Documentation Reconciliation](#documentation-reconciliation))
- [ ] **3. Write release notes**: Create `docs/release_notes/vX.Y.x/vX.Y.Z.md` from the change inventory
- [ ] **4. `VERSION`**: Set to `vX.Y.Z`
- [ ] **5. Sync Python files**: Update `protocol/python/pyproject.toml` (`version = "X.Y.Z"`), `protocol/python/g8e/__init__.py` (`__version__ = "X.Y.Z"`), and the editable `g8e` package entry in `protocol/python/uv.lock` (`version = "X.Y.Z"`) to match `VERSION` (no `v` prefix). This is required so CI's version sync check passes and `uv run --locked` remains usable on the PR. The agent edits these files manually; it cannot run `make release` to do it. `make release` re-syncs them after merge as a no-op safety net.
- [ ] **6. `CHANGELOG.md`**: Add a table row to the major-version section (no `v` prefix in version column)
- [ ] **7. Documentation headers (verify)**: Confirm that every doc modified in step 2 carries the new `Version:`/`Last Updated:` headers, and that no doc *not* modified in step 2 was bumped. Use `git diff --name-only <prev-tag>..HEAD -- docs/ protocol/docs/` to identify the modified set. Do not blanket-bump all headers.
- [ ] **8. Run [Verification](#verification)** to catch any missed files — including stale version callouts (step 8 of Verification)
- [ ] **9. Hand off**: The agent stops here. It does NOT `git add`, `git commit`, `git push`, open a PR, or run `make release`. The prepared working tree is handed back to the release owner.
- [ ] **10. Release owner commits and opens PR**: `git add -A && git commit -m "release: vX.Y.Z"`, push, and open a PR on GitHub. CI runs lint, tests, and version sync checks.
- [ ] **11. Release owner merges and releases**: After the PR is merged and CI on `main` passes, the release owner pulls main and runs `make release` to re-sync the Python package files (no-op if already synced), tag, and push; GitHub Actions workflows create the release and upload assets.

**5 files need manual version edits during PR prep** (VERSION + CHANGELOG + pyproject.toml + `__init__.py` + `uv.lock`), all made by the agent. `make release` re-syncs the Python files after merge as a no-op safety net and handles tagging/pushing — the release owner runs it, never the agent. Everything else is content-driven work: inventory, docs, and release notes.

> **Workflow note:** All release prep (steps 1-9) happens on a feature branch. The agent does steps 1-8 and stops; it does not commit, push, or open the PR. The release owner does steps 10-11 (commit, push, open PR, merge, wait for CI, pull main, run `make release`). GitHub Actions workflows handle release creation and asset uploads.

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

# 4. Verify Python package version matches VERSION. The agent syncs these files
#    manually during PR prep (step 5 of the Manual Updates Checklist) so CI's
#    version sync check passes on the PR. All three must show X.Y.Z matching RELEASE_NUM.
grep -n '^version' protocol/python/pyproject.toml
grep -n '__version__' protocol/python/g8e/__init__.py
grep -A1 '^name = "g8e"' protocol/python/uv.lock
# All three should show X.Y.Z matching RELEASE_NUM.

# 5. Find any doc version header (plain, bold, or "Document Version") NOT on the new
#    version; should return nothing for docs modified in this release. Docs NOT modified
#    are expected to still show the old version (that's the point). To check only modified
#    docs, pipe the git diff list:
#      git diff --name-only <prev-tag>..HEAD -- docs/ protocol/docs/ | xargs grep -niE '^(\*\*)?(document )?version:'
#    and verify each shows the new version. Unmodified docs are expected to lag.
grep -rniE '^(\*\*)?(document )?version:' docs/ protocol/docs/ --include='*.md' --exclude-dir=release_notes \
  | grep -viE "v?${RELEASE_NUM}([^0-9]|$)"

# 6. Find any "Last Updated" header not on the release date; should return nothing,
#    or only intentional entries (e.g., docs/devs/ docs on their own cadence, or docs
#    not modified in this release which are expected to keep their old date).
grep -rniE '^(\*\*)?last updated:' docs/ protocol/docs/ --include='*.md' --exclude-dir=release_notes \
  | grep -v "$RELEASE_DATE"

# 7. Verify no stale references remain; grep for identifiers that were removed or
#    renamed in this release and confirm no docs still reference them.
grep -rnE 'OldName|old_command|OLD_CONSTANT' docs/ protocol/docs/ --include='*.md'

# 8. Find any explicit g8e version callouts in docs that still reference a
#    previous release version. These are install/fetch commands like
#    `go get github.com/g8e-ai/g8e/v2@vX.Y.Z`, `pip install g8e==X.Y.Z`, and
#    `pip download g8e==X.Y.Z`. Every such callout must be updated to the new
#    release version in every release, regardless of whether the doc was touched
#    by a code change. Should return nothing. Third-party tool version pins
#    (e.g., `go install github.com/bufbuild/buf/cmd/buf@v1.70.0`) are not g8e
#    callouts and are excluded by the pattern.
grep -rnE "g8e==[0-9]+\.[0-9]+\.[0-9]+|g8e-ai/g8e/v2@v[0-9]+\.[0-9]+\.[0-9]+" docs/ protocol/docs/ README.md protocol/README.md --include='*.md' \
  | grep -v release_notes | grep -v CHANGELOG \
  | grep -viE "v?${RELEASE_NUM}([^0-9]|$)"
```

If step 4 shows a mismatch, the Python files were not synced during PR prep — fix them manually (set all three version entries to `X.Y.Z` matching `VERSION`) before handing off to the release owner. Steps 5 and 6 will show old versions/dates for docs not modified in this release; that is expected and correct. Only investigate results for docs that *were* modified in this release (per the git diff). Step 7 should return nothing; if it finds stale references, fix them before committing. Step 8 should return nothing; any stale version callout must be updated to the new release version before committing, and the doc's `Version:`/`Last Updated:` headers bumped in the same pass.

---

## Release Workflow

The `make release` target handles version syncing, tagging, and pushing in a single step. CI handles lint, tests, and version sync verification on PRs. GitHub Actions workflows handle release creation and asset uploads.

### `make release`: Tag and Push

> **Run this on the merged main branch**, not on a feature branch. The tags must point at the merge commit on main. **`make release` is run by the release owner, not by the agent preparing the PR.** The agent's work ends at opening the PR; the release owner merges, waits for CI on main to pass, pulls main locally, and then runs `make release`.

1. Syncs `protocol/python/pyproject.toml`, `protocol/python/g8e/__init__.py`, and the editable `g8e` package entry in `protocol/python/uv.lock` from `VERSION` (if already in sync, no changes are made)
2. Verifies working tree is clean (fails if Python files were out of sync; commit synced files and go through the PR process first)
3. Verifies release notes file exists at `docs/release_notes/vX.Y.x/vX.Y.Z.md`
4. Verifies tags `vX.Y.Z` and `protocol/vX.Y.Z` don't already exist
5. Creates `vX.Y.Z` and `protocol/vX.Y.Z` tags on the current commit
6. Pushes both tags to origin

The `vX.Y.Z` tag triggers the `release-binary.yml` workflow, which builds all platforms, signs binaries, creates the GitHub release, and uploads assets. The `protocol/vX.Y.Z` tag triggers the `release-python-protocol.yml` workflow, which publishes the Python package to PyPI.

> **Lint and tests are handled by CI** (`.github/workflows/build-and-test.yml`) on pull requests, not by `make release`. The CI workflow includes a version sync check that fails if `pyproject.toml`, `__init__.py`, or the editable `g8e` entry in `uv.lock` doesn't match `VERSION`.

### CI Workflows Triggered by Tags

| Tag | Workflow | What It Does |
|-----|----------|-------------|
| `vX.Y.Z` | `.github/workflows/release-binary.yml` | Builds all platforms, signs binaries with cosign, uploads assets to GitHub release, and verifies fresh `go install` works on Ubuntu, macOS, and Windows |
| `protocol/vX.Y.Z` | `.github/workflows/release-python-protocol.yml` | Builds and publishes Python package to PyPI, verifies fresh PyPI install and imports on Ubuntu, macOS, and Windows |

The `protocol/v*` tag is used only as a trigger for the Python PyPI release workflow. It is NOT used for Go module versioning; the Go module is part of the root module and is versioned by `v*` tags.

---

## Emergency Releases (Hotfixes)

For critical security issues or production bugs:

1. Apply the minimal fix necessary to the appropriate branch
2. Inventory the changes and reconcile documentation for the touched areas
3. Set `VERSION`, sync the Python package files to match, update `CHANGELOG.md`, and write release notes
4. Hand off to the release owner, who commits, pushes, opens and merges the PR; after CI on `main` passes, pulls main locally, and runs `make release` to re-sync the Python files (no-op), tag, and push; GitHub Actions workflows create the release and upload assets

---

## Out of Scope: Manual Git Steps

The only git operations not automated by `make release` are:

- Staging and committing the release-prep changes (`git add`, `git commit`) — release owner
- Pushing the branch and opening a PR (`git push`, GitHub PR) — release owner
- Merging the PR on GitHub — release owner
- Pulling main after merge (`git checkout main && git pull`) — release owner

`make release` handles tag creation and tag pushing automatically; the release owner runs it on the merged main branch after CI passes. GitHub Actions workflows handle release creation and asset uploads. The agent does not run `git commit`, `git push`, `make release`, or any other mutating git/make command — it prepares the working tree and hands it back.

---

## References

- [Semantic Versioning](https://semver.org/spec/v2.0.0.html)
- [Keep a Changelog](https://keepachangelog.com/en/1.0.0/)
- [GitHub Releases Documentation](https://docs.github.com/en/repositories/releasing-projects-on-github/managing-releases-in-a-repository)
- [Go Module Versioning](https://go.dev/doc/modules/versioning)
- [PyPI Packaging](https://packaging.python.org/en/latest/tutorials/packaging-projects/)
