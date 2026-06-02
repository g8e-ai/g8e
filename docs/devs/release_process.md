# g8e Release Process

This document describes the complete release process for g8e, covering all steps required after code changes are complete and tests are passing.

## Prerequisites

Before starting a release, ensure:

- All code changes for the release are merged to the main branch
- All tests pass locally: `make ci` or `make ci-platform`
- No outstanding security vulnerabilities: `make vulncheck`
- All linting passes: `make lint`
- Protocol generation is up to date: `make proto` (no uncommitted changes to generated .pb.go files)
- Documentation is updated for any breaking changes or new features

## Pre-Release Preparation

Complete all of the following steps before creating the git tag. These steps prepare the repository for release but do not yet publish anything.

### 1. Determine Version Number

Follow [Semantic Versioning](https://semver.org/spec/v2.0.0.html):

- **MAJOR** (X.0.0): Breaking changes that require user action
- **MINOR** (x.Y.0): New features, backward-compatible changes
- **PATCH** (x.y.Z): Bug fixes, backward-compatible changes

Check the current version:
```bash
cat VERSION
```

### 2. Update VERSION File

Update the VERSION file with the new version:

```bash
echo "v1.0.5" > VERSION
```

The VERSION file must contain only the version string with a newline (no trailing spaces).

### 3. Update CHANGELOG.md

Add a new section at the top of CHANGELOG.md following the [Keep a Changelog](https://keepachangelog.com/en/1.0.0/) format:

```markdown
## [1.0.5] - YYYY-MM-DD

### Overview

[Brief summary of the release - 2-3 sentences highlighting major changes]

### Breaking Changes

* **Change title** - Description of breaking change and migration path
* **Another breaking change** - Description and impact

### Added

* **Feature title** - Description of new feature
* **Another feature** - Description

### Changed

* **Change title** - Description of modification
* **Another change** - Description

### Fixed

* **Bug title** - Description of fix
* **Another bug** - Description

### Security

* **Security title** - Description of security improvement
```

**Guidelines:**
- Group changes under appropriate headers
- Use concise, clear descriptions
- Reference relevant files or components where helpful
- Include migration instructions for breaking changes
- Date format: YYYY-MM-DD

### 4. Create Release Notes File

Create a new release notes file in `docs/release_notes/`:

```bash
touch docs/release_notes/v1.0.5.md
```

The release notes file should mirror the CHANGELOG entry but can be more detailed. Include:

- Version header and date
- Overview section
- All sections from CHANGELOG (Breaking Changes, Added, Changed, Fixed, Security)
- Additional context or examples if helpful
- Links to relevant documentation

**Example structure:**
```markdown
## [1.0.5] - YYYY-MM-DD

### Overview

v1.0.5 introduces [major feature], adds [another feature], and fixes [critical bug]. This release focuses on [theme].

### Breaking Changes

* **Change title** - Description with migration path
  ```bash
  # Migration example
  ./g8e old-command → ./g8e new-command
  ```

### Added

* **Feature title** - Detailed description
* **Another feature** - Description with usage example

### Changed

* **Change title** - Description
* **Another change** - Description

### Fixed

* **Bug title** - Description
* **Another bug** - Description

### Security

* **Security title** - Description
```

### 5. Update Protocol Version (if applicable)

If protocol changes are included in this release, update the protocol version:

**Go Protocol:**
- The protocol module is at `protocol/go.mod` (module: `github.com/g8e-ai/g8e/protocol`)
- The protocol version follows the platform version but may be released independently
- No separate version field in go.mod - version is managed via git tags

**Python Protocol:**
- Update `protocol/python/pyproject.toml` version (line 20: `version = "X.Y.Z"`)
- The package name is `g8e-protocol` (not `g8e_protocol`)

**Note:** Protocol packages are released via GitHub tags with the format `protocol/vX.Y.Z`. These can be released independently of platform releases.

### 6. Build and Verify

Build the binary and verify it works:

```bash
# Build the binary
make build

# Verify the binary
./g8e --help

# Run tests
make test

# Run integration tests (if applicable)
make test-integration
```

**Note:** The build command outputs a single binary named `g8e` (not `g8e operator`). The build process embeds version information via ldflags from the VERSION file.

Check that the build includes the correct version:
```bash
./g8e -v
# or
./g8e --version
```

### 7. Commit Changes

Commit all release-related changes:

```bash
git add VERSION CHANGELOG.md docs/release_notes/v1.0.5.md
git commit -m "Release v1.0.5"
```

**Commit message guidelines:**
- Use the version number as the commit message
- Include a brief summary in the commit body if needed
- Reference relevant issue numbers if applicable

### 8. Final Pre-Release Verification

Before proceeding to the actual release, verify everything is in order:

```bash
# Verify VERSION file is correct
cat VERSION

# Verify CHANGELOG is updated
head -n 50 CHANGELOG.md

# Verify release notes file exists
ls docs/release_notes/v1.0.5.md

# Verify no uncommitted changes
git status

# Verify the commit is on main branch
git branch --show-current
```

Ensure all pre-release changes are committed and the working directory is clean before proceeding to the release execution phase.

## Release Execution

Once all pre-release preparation is complete and committed, execute the following steps to publish the release.

### 1. Create Git Tag

Create and push an annotated tag for the release:

```bash
# Create annotated tag
git tag -a v1.0.5 -m "Release v1.0.5"

# Push the commit and tag
git push origin main
git push origin v1.0.5
```

**Tag guidelines:**
- Use annotated tags (not lightweight tags)
- Tag format: `vX.Y.Z` (matches VERSION file without 'v' prefix)
- Include release notes in the tag message or reference the release notes file

### 2. Create GitHub Release

Create a GitHub release via the web interface or GitHub CLI:

**Via GitHub CLI:**
```bash
gh release create v1.0.5 \
  --title "Release v1.0.5" \
  --notes-file docs/release_notes/v1.0.5.md
```

**Via Web Interface:**
1. Go to GitHub repository → Releases
2. Click "Draft a new release"
3. Tag: Select `v1.0.5`
4. Title: `Release v1.0.5`
5. Description: Copy contents from `docs/release_notes/v1.0.5.md`
6. Attach binaries if distributing via GitHub Releases (optional)
7. Click "Publish release"

**Release notes guidelines:**
- Use the release notes file content
- Include installation instructions
- Highlight breaking changes prominently
- Link to full CHANGELOG for detailed changes

### 3. Release Protocol Packages (if applicable)

If protocol changes are included, release the Go and Python protocol packages.

#### Version Synchronization

The Go and Python packages must use the same version number. Update the Python version in `protocol/python/pyproject.toml` before tagging. The Go module version derives from the git tag.

- Go: `protocol/go.mod` - version determined by git tag
- Python: `protocol/python/pyproject.toml` - version field in `[project]` section

#### Tag and Push

```bash
# Update Python version first
# protocol/python/pyproject.toml: version = "X.Y.Z"

# Tag the protocol release
git tag -a protocol/v1.0.5 -m "Protocol v1.0.5"
git push origin protocol/v1.0.5
```

The tag format `protocol/vX.Y.Z` triggers both release workflows.

#### Automated Workflows

**Go Protocol** (`.github/workflows/release-go-protocol.yml`):
- Runs tests with Go 1.26
- Creates GitHub release with installation instructions
- Publishes Go module

**Python Protocol** (`.github/workflows/release-python-protocol.yml`):
- Runs tests with Python 3.14
- Builds package using standard `build` module
- Validates package with `twine check`
- Publishes to PyPI using `PYPI_API_TOKEN`
- Creates GitHub release with installation instructions

#### Verification

After workflows complete, verify both packages:

```bash
# Go verification
go get github.com/g8e-ai/g8e/protocol@v1.0.5

# Python verification
pip install g8e-protocol==1.0.5
```

### 4. Update Documentation

Update any documentation that references version numbers:

- **README.md**: Update status section if the version changes significantly
- **Getting Started Guide**: Update any version-specific instructions
- **API Reference**: Update if API changes are included
- **Architecture Docs**: Update if architectural changes are included

### 5. Post-Release Verification

After the release is published:

1. **Verify GitHub Release**: Check that the release is visible and notes are correct
2. **Verify Protocol Packages**: Check that Go and Python packages are available
  - Go: `go list -m -versions github.com/g8e-ai/g8e/protocol`
  - Python: `pip index versions g8e-protocol`
3. **Test Installation**: Verify users can install the new version
  ```bash
  # Test Go protocol installation
  go get github.com/g8e-ai/g8e/protocol@v1.0.5

  # Test Python protocol installation
  pip install g8e-protocol==1.0.5
  ```
4. **Monitor Issues**: Watch for any post-release issues or regressions

### 6. Announce the Release

Announce the release through appropriate channels:

- Update project status in README if applicable
- Post release notes to project communication channels
- Notify stakeholders of breaking changes
- Update project roadmaps or milestones

## Emergency Releases (Hotfixes)

For critical security issues or production bugs, follow this expedited process:

1. Create a release branch from the previous release tag:
   ```bash
   git checkout -b release/v1.0.6 v1.0.5
   ```

2. Apply the minimal fix necessary

3. Update VERSION, CHANGELOG, and create release notes

4. Commit, tag, and release as usual

5. Merge the hotfix back to main:
   ```bash
   git checkout main
   git merge release/v1.0.6
   ```

## Version Compatibility

- **Platform Version**: Tracked in `VERSION` file (e.g., `v1.0.5`)
- **Go Protocol Version**: Managed via git tags (module: `github.com/g8e-ai/g8e/protocol`)
- **Python Protocol Version**: Tracked in `protocol/python/pyproject.toml` (package: `g8e-protocol`)

Protocol versions may be released independently of platform versions. However, for major platform releases, coordinate protocol releases to ensure compatibility.

## Rollback Procedure

If a critical issue is discovered after release:

1. Identify the last known good version
2. Communicate the issue to users
3. If necessary, yank the release from package registries:
   - PyPI: `pip yank g8e-protocol==X.Y.Z`
   - Go: Cannot yank, but can deprecate in next release
4. Release a new version with the fix
5. Update documentation to guide users to the safe version

## References

- [Semantic Versioning](https://semver.org/spec/v2.0.0.html)
- [Keep a Changelog](https://keepachangelog.com/en/1.0.0/)
- [GitHub Releases Documentation](https://docs.github.com/en/repositories/releasing-projects-on-github/managing-releases-in-a-repository)
- [Go Module Versioning](https://go.dev/doc/modules/versioning)
- [PyPI Packaging](https://packaging.python.org/en/latest/tutorials/packaging-projects/)
