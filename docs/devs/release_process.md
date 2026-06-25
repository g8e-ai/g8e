# g8e Release Process — Version & Documentation Updates

This document is the definitive checklist for updating all version-bearing files and documentation when bumping a g8e release. Every file listed here must be updated, verified, and committed before a git tag is created.

## How to Use This Document

1. Determine the new version number (see [Versioning Rules](#versioning-rules))
2. Work through the [Version-Bearing Files](#version-bearing-files) section top-to-bottom
3. Run the [Verification](#verification) section to catch any missed files
4. Commit all changes together as a single release-prep commit

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

Throughout this document, **`vX.Y.Z`** refers to the new release version (e.g., `v1.2.2`), and **`YYYY-MM-DD`** refers to the release date.

---

## Version-Bearing Files

The following files contain version strings that must be updated on every release. They are grouped by category.

### A. Core Version Files

These are the canonical files that define the platform and protocol versions.

| # | File | What to Update | Format |
|---|------|---------------|--------|
| 1 | `VERSION` | Entire file contents | `vX.Y.Z\n` (with trailing newline, no trailing spaces) |
| 2 | `CHANGELOG.md` | Add new section at top (after `## [Unreleased]` block) | `## [X.Y.Z] - YYYY-MM-DD` with Overview, Breaking Changes, Added, Changed, Fixed, Security subsections |
| 3 | `protocol/python/pyproject.toml` | `version` field in `[project]` section (line 20) | `version = "X.Y.Z"` (no `v` prefix) |

#### CHANGELOG.md Template

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

### Fixed

* **Bug title** - Description of fix

### Security

* **Security title** - Description of security improvement
```

### B. Release Notes File

Create a new release notes file for every release.

| # | File | Action |
|---|------|--------|
| 4 | `docs/release_notes/vX.Y.x/vX.Y.Z.md` | Create new file (use the minor-version subdirectory, e.g., `v1.2.x/v1.2.2.md`) |

The release notes file should mirror the CHANGELOG entry but can be more detailed. Include:

- Version header and date
- Overview section
- All sections from CHANGELOG (Breaking Changes, Added, Changed, Fixed, Security)
- Additional context or examples if helpful
- Links to relevant documentation

**Example structure:**

```markdown
## [X.Y.Z] - YYYY-MM-DD

### Overview

vX.Y.Z introduces [major feature], adds [another feature], and fixes [critical bug]. This release focuses on [theme].

### Breaking Changes

* **Change title** - Description with migration path

### Added

* **Feature title** - Detailed description

### Changed

* **Change title** - Description

### Fixed

* **Bug title** - Description

### Security

* **Security title** - Description
```

### C. Documentation with `Version:` Headers

Every markdown file below has a `Version: vX.Y.Z` header (and a `Last Updated: YYYY-MM-DD` date) near the top. **Both fields must be updated** on every release to match the new version and release date.

#### Architecture Docs (`docs/architecture/`)

| # | File |
|---|------|
| 5 | `docs/architecture/auth.md` |
| 6 | `docs/architecture/binding.md` |
| 7 | `docs/architecture/encryption.md` |
| 8 | `docs/architecture/gateway.md` |
| 9 | `docs/architecture/network.md` |
| 10 | `docs/architecture/operator.md` |
| 11 | `docs/architecture/postures.md` |
| 12 | `docs/architecture/sse.md` |
| 13 | `docs/architecture/storage.md` |
| 14 | `docs/architecture/transaction-process.md` |

#### Guide Docs (`docs/guides/`)

| # | File |
|---|------|
| 15 | `docs/guides/air_gap.md` |
| 16 | `docs/guides/build_apps.md` |
| 17 | `docs/guides/build_gateway.md` |
| 18 | `docs/guides/build_operator.md` |
| 19 | `docs/guides/cli.md` |
| 20 | `docs/guides/connect_apps_to_gateway.md` |
| 21 | `docs/guides/connect_operator_to_gateway.md` |
| 22 | `docs/guides/docker_gateway.md` |
| 23 | `docs/guides/getting_started.md` |

#### Reference Docs (`docs/reference/`)

| # | File |
|---|------|
| 24 | `docs/reference/glossary.md` |

#### Protocol Docs (`protocol/docs/`)

| # | File |
|---|------|
| 25 | `protocol/docs/spec.md` |

#### Update Pattern for Each Doc

For each file above, update the two header lines:

```diff
-Last Updated: 2026-06-24
-Version: v1.2.0
+Last Updated: YYYY-MM-DD
+Version: vX.Y.Z
```

### D. Docs Without Version Headers (No Version Update Required)

The following doc directories intentionally do **not** carry `Version:` headers and do **not** need version updates on release. They may still need content updates if the release changes their subject matter:

- `docs/core/` — Position papers, about page
- `docs/devs/` — Developer documentation (including this file)
- `docs/diagrams/` — Mermaid diagrams and flowcharts
- `docs/release_notes/` — Historical release notes (past entries are immutable)
- `demos/` — Demo configurations and doctrine files
- `README.md` — Uses a dynamic GitHub badge for latest release; no hardcoded version

### E. Go Protocol Module

| # | File | Notes |
|---|------|-------|
| — | `protocol/go.mod` | No version field to update. The Go module version is derived from git tags (`protocol/vX.Y.Z`). |

### F. Build & CI Files (No Version Update Required)

The following files read the version dynamically from `VERSION` at build time and do **not** require manual updates:

- `Makefile` — Reads `VERSION` via `$(shell cat VERSION)`
- `Dockerfile` / `Dockerfile.operator` — Receive version as build arg
- `docker-compose.yml` — References build context, not version
- `.github/workflows/*.yml` — Triggered by git tags, no hardcoded version

---

## Complete Update Checklist

When preparing a release, work through this checklist in order:

- [ ] **1. `VERSION`** — Set to `vX.Y.Z`
- [ ] **2. `CHANGELOG.md`** — Add `## [X.Y.Z] - YYYY-MM-DD` section
- [ ] **3. `protocol/python/pyproject.toml`** — Update `version = "X.Y.Z"` (no `v` prefix)
- [ ] **4. `docs/release_notes/vX.Y.x/vX.Y.Z.md`** — Create new release notes file
- [ ] **5–14. Architecture docs** — Update `Version:` and `Last Updated:` in all 10 files under `docs/architecture/`
- [ ] **15–23. Guide docs** — Update `Version:` and `Last Updated:` in all 9 files under `docs/guides/`
- [ ] **24. `docs/reference/glossary.md`** — Update `Version:` and `Last Updated:`
- [ ] **25. `protocol/docs/spec.md`** — Update `Version:` and `Last Updated:`

**Total: 25 files to update on every release.**

---

## Verification

After making all updates, run these checks to catch any missed files:

```bash
# 1. Verify VERSION file is correct
cat VERSION

# 2. Verify CHANGELOG has the new version section
head -n 50 CHANGELOG.md

# 3. Verify release notes file exists
ls docs/release_notes/vX.Y.x/vX.Y.Z.md

# 4. Verify Python protocol version matches
grep '^version' protocol/python/pyproject.toml

# 5. Find any docs with stale Version headers (should return nothing)
RELEASE_VERSION=$(cat VERSION)
grep -rn '^Version: v' docs/ protocol/docs/ --include='*.md' | grep -v "$RELEASE_VERSION"

# 6. Find any docs with stale Last Updated dates (should return nothing or only intentional entries)
RELEASE_DATE="YYYY-MM-DD"
grep -rn '^Last Updated:' docs/ protocol/docs/ --include='*.md' | grep -v "$RELEASE_DATE"
```

If step 5 returns any results, those files still have an old version — update them before proceeding.

---

## Commit

Commit all release-related changes as a single commit:

- **Files**: All 25 files listed above
- **Commit message**: Use the version number (e.g., `v1.2.2`)
- Include a brief summary in the commit body if needed

---

## Protocol Release Notes (If Applicable)

Protocol packages (Go and Python) may be released independently of platform releases. If protocol changes are included:

1. The Go and Python packages must use the same version number
2. The Go module version derives from the git tag (`protocol/vX.Y.Z`)
3. The Python version is set in `protocol/python/pyproject.toml` (already covered in the checklist above)
4. The `protocol/vX.Y.Z` tag triggers both release workflows:
   - **Go**: `.github/workflows/release-go-protocol.yml`
   - **Python**: `.github/workflows/release-python-protocol.yml`

---

## Emergency Releases (Hotfixes)

For critical security issues or production bugs:

1. Apply the minimal fix necessary to the appropriate branch
2. Work through the [Complete Update Checklist](#complete-update-checklist) above
3. Proceed with the standard tag and release process

---

## References

- [Semantic Versioning](https://semver.org/spec/v2.0.0.html)
- [Keep a Changelog](https://keepachangelog.com/en/1.0.0/)
- [GitHub Releases Documentation](https://docs.github.com/en/repositories/releasing-projects-on-github/managing-releases-in-a-repository)
- [Go Module Versioning](https://go.dev/doc/modules/versioning)
- [PyPI Packaging](https://packaging.python.org/en/latest/tutorials/packaging-projects/)
