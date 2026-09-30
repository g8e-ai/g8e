---
doc_id: release_process
title: Release Process
audience: maintainers and coding agents
status: current
last_updated: 2026-09-30
version: v2.2.5
owners:
  - VERSION
  - Makefile
  - CHANGELOG.md
  - protocol/python/pyproject.toml
  - protocol/python/g8e/__init__.py
  - protocol/python/uv.lock
  - protocol/docs/
  - internal/cli/cmd/compliance/
related:
  - docs/devs/devs.md
  - docs/devs/codemap.md
  - docs/devs/tests.md
  - docs/devs/docs.md
  - docs/devs/troubleshooting.md
when_to_read: Preparing, auditing, generating compliance evidence for, and executing a platform release.
do_not_use_for:
  - Coding invariants and style guidelines (docs/devs/devs.md)
  - Documentation audit workflow and documentation catalog (docs/devs/docs.md)
  - Test tiers and test execution (docs/devs/tests.md)
  - Runtime and package ownership maps (docs/devs/codemap.md)
---

# Release Process

## Purpose

Defines the release preparation, change inventory, documentation reconciliation, version synchronization, compliance evidence generation, and release workflow for the g8e platform and protocol packages.

## Quick index

- [Purpose](#purpose)
- [Invariants](#invariants)
- [Owned surfaces](#owned-surfaces)
- [Procedures](#procedures)
- [Anti-patterns](#anti-patterns)
- [Links out](#links-out)

Invariant groups: [Separation of duties](#separation-of-duties-inv-rel-duty), [Version synchronization](#version-synchronization-inv-rel-ver), [Compliance evidence](#compliance-evidence-inv-rel-comp).

## Invariants

Ids are stable. Append the next free number in a topic. Do not renumber.

### Separation of duties (`INV-REL-DUTY`)

| ID | Rule |
| --- | --- |
| INV-REL-DUTY-01 | The AI coding agent MUST NOT run `git commit`, `git push`, `gh pr create`, or `make release`. The agent prepares the working tree, audits docs, generates compliance evidence, and hands back the prepared tree to the release owner. |
| INV-REL-DUTY-02 | The release owner MUST review prepared changes, commit, open and merge the PR, pull the merged `main` branch locally, and execute `make release` to tag and push. |

### Version synchronization (`INV-REL-VER`)

| ID | Rule |
| --- | --- |
| INV-REL-VER-01 | `VERSION` is the single source of truth for the platform and protocol version (`vX.Y.Z\n`). |
| INV-REL-VER-02 | PR preparation MUST synchronize `protocol/python/pyproject.toml`, `protocol/python/g8e/__init__.py`, the editable package entry in `protocol/python/uv.lock`, and the `Version: vX.Y.Z` headers in `protocol/docs/a2a.md`, `protocol/docs/constants.md`, `protocol/docs/mcp.md`, and `protocol/docs/spec.md`. |
| INV-REL-VER-03 | `make proto` MUST be run during release PR prep to regenerate downstream `ensemble/uv.lock` and protocol bindings so locked CI checks pass. |
| INV-REL-VER-04 | `CHANGELOG.md` MUST include a row under the minor-version section (`## vX.Y.x`) linking to the new release notes file `docs/release_notes/vX.Y.x/vX.Y.Z.md`. |

### Compliance evidence (`INV-REL-COMP`)

| ID | Rule |
| --- | --- |
| INV-REL-COMP-01 | Every release MUST include a signed, verified public compliance report bundle and its canonical projections (`vX.Y.Z-compliance-evidence.md` and `vX.Y.Z-compliance-evidence.csv`) in `docs/release_notes/vX.Y.x/`. |
| INV-REL-COMP-02 | Report trust policies and evidence trust policies MUST be external to the report bundle. Supplying external trust does not replace first-party engineering assessment. |
| INV-REL-COMP-03 | Compliance projections MUST match the canonical renderings in the bundle (`report.md` and `report.csv`). |

## Owned surfaces

| Claim | Path | Verify |
| --- | --- | --- |
| Repository version | `VERSION` | `cat VERSION` |
| Release index | `CHANGELOG.md` | Table row under `## vX.Y.x` |
| Python protocol version | `protocol/python/pyproject.toml`, `protocol/python/g8e/__init__.py`, `protocol/python/uv.lock` | Version match with `VERSION` |
| Protocol spec headers | `protocol/docs/a2a.md`, `constants.md`, `mcp.md`, `spec.md` | `Version: vX.Y.Z` header |
| Downstream lockfile | `ensemble/uv.lock` | Matches `g8e` version |
| Compliance projection | `docs/release_notes/vX.Y.x/vX.Y.Z-compliance-evidence.md`, `.csv` | `g8e compliance release-evidence` |
| Tag and push target | `Makefile` (`make release`) | Release owner only on merged `main` |

## Procedures

### Agent PR Preparation Sequence

1. **Determine change inventory:** Diff `v<prev-tag>..HEAD` to identify all added, changed, removed, fixed, and security-sensitive features.
2. **Reconcile documentation:** Walk the documentation catalog in `docs/devs/docs.md`, audit affected documents end-to-end against production code, and update stale cross-links.
3. **Draft release notes:** Create `docs/release_notes/vX.Y.x/vX.Y.Z.md` following the template with Overview, Added, Changed, Fixed, Tests, Documentation, Compliance Evidence, and Deferred sections.
4. **Set version:** Update `VERSION` to `vX.Y.Z\n`.
5. **Sync Python package & protocol specs:** Update `protocol/python/pyproject.toml`, `protocol/python/g8e/__init__.py`, `protocol/python/uv.lock`, and the `Version: vX.Y.Z` headers in `protocol/docs/a2a.md`, `protocol/docs/constants.md`, `protocol/docs/mcp.md`, and `protocol/docs/spec.md`.
6. **Regenerate downstream lockfiles:** Run `make proto` to update `ensemble/uv.lock` and protobuf bindings.
7. **Update CHANGELOG:** Add the release table row under `## vX.Y.x` in `CHANGELOG.md`.
8. **Generate and verify compliance evidence:** Copy the running Gateway database to `<runtime-root>/.g8e/data/g8e.db` (`docker cp` the `g8e.db`, `-wal`, and `-shm` files from the Gateway container), then run `./g8e compliance release-prepare --project-root <runtime-root> --signing-metadata <path> --signing-private-key <path>` (add `--new-key` to create the signing key). The command derives the protected scope with `ProductVersion = "X.Y.Z"` from `VERSION`, git, and the Gateway image digest, exports the assessment window, builds the external report and evidence trust policies, generates a public bundle, verifies it offline, and projects the verified `report.md` and `report.csv` into `docs/release_notes/vX.Y.x/`. The recorded source revision is the git `HEAD` of `--repo-root`, so the running Gateway MUST be built from that `HEAD` (`make build`, then `./g8e docker rebuild --full`; confirm with `g8e version` inside the Gateway container) and the assessment window MUST contain governed activity from that build; the automatic window fails when it holds no receipts. Uncommitted release changes are never part of the recorded revision. The command stops before projection when verification fails. Run `./g8e compliance report generate`, `report verify`, and `release-evidence` individually only for a scope authored by hand. Record the bundle path, verification result, and checksum root that the command prints in the release notes.
9. **Finalize document metadata:** Update `last_updated` and `version` on every audited document.
10. **Verification checks:** Run `./g8e test lint` and focused package integration tests.

### Release Owner Tagging and Publication (Post-Merge)

```bash
# On merged main branch with clean tree:
git checkout main && git pull
make release
```

`make release` rewrites any out-of-sync Python package version or protocol spec header to match `VERSION`, then aborts if that leaves the working tree dirty. It also aborts when the release notes file is missing or either tag already exists. Otherwise it creates tags `vX.Y.Z` and `protocol/vX.Y.Z` and pushes them to origin. GitHub Actions workflows build binaries, sign assets, create the GitHub release, and publish to PyPI.

## Anti-patterns

- Agent running `git commit`, `git push`, or `make release` (INV-REL-DUTY-01).
- Running `make release` from a feature branch or with unmerged changes (INV-REL-DUTY-02).
- Updating document metadata without auditing the entire document against code (INV-DOC-FMT-02 in [Documentation Guide](docs.md)).
- Recording a source revision for a Gateway that was built from a different commit (INV-REL-COMP-01).
- Hand-editing compliance projections instead of generating through `g8e compliance release-evidence` (INV-REL-COMP-01, INV-REL-COMP-03).
- Missing `make proto` to update `ensemble/uv.lock` after bumping Python version (INV-REL-VER-03).

## Links out

- [Developer Guidelines](devs.md): coding invariants and repository standards.
- [Documentation Guide](docs.md): documentation audit, catalog, and formatting standards.
- [Testing Guide](tests.md): test execution tiers and verification commands.
- [Code Map](codemap.md): package and runtime ownership maps.
